package capture

import (
	"context"
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// CaptureAudio taps the sound that process pid and its children play, and calls
// emit with each packet and the time it was played, until ctx ends. It is a
// copy of the stream in shared mode: the process keeps playing to the speakers
// as before, and no other program's sound is included. It needs Windows 10
// version 2004 or later.
func CaptureAudio(ctx context.Context, pid uint32, emit func(pcm []byte, at time.Time) error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_MULTITHREADED); err != nil {
		return fmt.Errorf("start COM: %w", err)
	}
	defer windows.CoUninitialize()

	client, err := activateProcessLoopback(pid)
	if err != nil {
		return err
	}
	defer comRelease(client)

	format := waveFormat{
		formatTag: 1, // PCM
		channels:  AudioChannels, samplesPerSec: AudioRate, bitsPerSample: AudioBits,
		blockAlign: AudioFrameBytes, avgBytesPerSec: AudioRate * AudioFrameBytes,
	}
	const bufferDuration = 2_000_000 // 200 ms in 100 ns units
	if hr := comCall(client, clientInitialize, 0, streamLoopback|streamEventCallback|streamAutoConvertPCM,
		bufferDuration, 0, uintptr(unsafe.Pointer(&format)), 0); failed(hr) {
		return hresultError("initialize audio client", hr)
	}
	ready, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(ready)
	if hr := comCall(client, clientSetEventHandle, uintptr(ready)); failed(hr) {
		return hresultError("set audio event", hr)
	}
	var capture unsafe.Pointer
	if hr := comCall(client, clientGetService, uintptr(unsafe.Pointer(&iidAudioCaptureClient)), uintptr(unsafe.Pointer(&capture))); failed(hr) {
		return hresultError("get capture client", hr)
	}
	defer comRelease(capture)
	if hr := comCall(client, clientStart); failed(hr) {
		return hresultError("start audio capture", hr)
	}
	defer comCall(client, clientStop)

	for ctx.Err() == nil {
		if _, err := windows.WaitForSingleObject(ready, 100); err != nil {
			return err
		}
		for {
			var data unsafe.Pointer
			var frames, flags uint32
			var devicePos, qpcPos uint64
			hr := comCall(capture, captureGetBuffer, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&frames)),
				uintptr(unsafe.Pointer(&flags)), uintptr(unsafe.Pointer(&devicePos)), uintptr(unsafe.Pointer(&qpcPos)))
			if failed(hr) {
				return hresultError("read audio", hr)
			}
			if hr == bufferEmpty || frames == 0 {
				break
			}
			at := time.Now().Add(-time.Duration(int64(now100ns())-int64(qpcPos)) * 100)
			pcm := make([]byte, int(frames)*AudioFrameBytes) // zeros when the packet is marked silent
			if flags&bufferSilent == 0 {
				copy(pcm, unsafe.Slice((*byte)(data), len(pcm)))
			}
			comCall(capture, captureReleaseBuffer, uintptr(frames))
			if err := emit(pcm, at); err != nil {
				return err
			}
		}
	}
	return nil
}

// WindowProcess returns the ID of the process that owns the top-level window
// with this exact title.
func WindowProcess(title string) (uint32, error) {
	name, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return 0, err
	}
	hwnd, _, _ := findWindow.Call(0, uintptr(unsafe.Pointer(name)))
	if hwnd == 0 {
		return 0, fmt.Errorf("no window titled %q", title)
	}
	var pid uint32
	if _, err := windows.GetWindowThreadProcessId(windows.HWND(hwnd), &pid); err != nil {
		return 0, err
	}
	return pid, nil
}

// WindowExe returns the full path of the exe that owns the window titled title.
func WindowExe(title string) (string, error) {
	pid, err := WindowProcess(title)
	if err != nil {
		return "", err
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf[:n]), nil
}

// activateProcessLoopback opens an audio client on the process loopback device.
func activateProcessLoopback(pid uint32) (unsafe.Pointer, error) {
	params := activationParams{activationType: 1, pid: pid} // process loopback, include the process tree
	blob := propVariant{vt: vtBlob, size: uint32(unsafe.Sizeof(params)), data: unsafe.Pointer(&params)}
	device, _ := windows.UTF16PtrFromString("VAD\\Process_Loopback")
	handler := &completionHandler{vtbl: &handlerVtbl, done: make(chan struct{})}
	var op unsafe.Pointer
	hr, _, _ := activateAudioInterfaceAsync.Call(uintptr(unsafe.Pointer(device)), uintptr(unsafe.Pointer(&iidAudioClient)),
		uintptr(unsafe.Pointer(&blob)), uintptr(unsafe.Pointer(handler)), uintptr(unsafe.Pointer(&op)))
	if failed(hr) {
		return nil, hresultError("activate process loopback", hr)
	}
	defer comRelease(op)
	<-handler.done // Windows always calls the handler, also on failure
	runtime.KeepAlive(handler)
	runtime.KeepAlive(&params)
	var result uintptr
	var client unsafe.Pointer
	if hr := comCall(op, opGetActivateResult, uintptr(unsafe.Pointer(&result)), uintptr(unsafe.Pointer(&client))); failed(hr) {
		return nil, hresultError("activate process loopback", hr)
	}
	if failed(result) {
		return nil, hresultError("activate process loopback", result)
	}
	return client, nil
}

// completionHandler is a minimal COM object that ActivateAudioInterfaceAsync
// calls when it is done. It lives in Go memory, which does not move.
type completionHandler struct {
	vtbl *[4]uintptr
	done chan struct{}
}

var handlerVtbl = [4]uintptr{
	syscall.NewCallback(func(this *completionHandler, iid *windows.GUID, out *unsafe.Pointer) uintptr {
		if *iid != iidUnknown && *iid != iidCompletionHandler && *iid != iidAgileObject {
			*out = nil
			return 0x80004002 // E_NOINTERFACE
		}
		*out = unsafe.Pointer(this)
		return 0
	}),
	syscall.NewCallback(func(*completionHandler) uintptr { return 1 }), // AddRef: Go owns the object
	syscall.NewCallback(func(*completionHandler) uintptr { return 1 }), // Release
	syscall.NewCallback(func(this *completionHandler, _ uintptr) uintptr {
		close(this.done)
		return 0
	}),
}

type activationParams struct {
	activationType uint32
	pid            uint32
	mode           uint32
}

// propVariant is a PROPVARIANT holding a BLOB.
type propVariant struct {
	vt   uint16
	_    [3]uint16
	size uint32
	data unsafe.Pointer
}

// waveFormat is WAVEFORMATEX. Windows reads its first 18 bytes.
type waveFormat struct {
	formatTag      uint16
	channels       uint16
	samplesPerSec  uint32
	avgBytesPerSec uint32
	blockAlign     uint16
	bitsPerSample  uint16
	cbSize         uint16
}

const (
	vtBlob               = 65
	streamLoopback       = 0x00020000
	streamEventCallback  = 0x00040000
	streamAutoConvertPCM = 0x80000000
	bufferSilent         = 0x2
	bufferEmpty          = 0x08890001 // AUDCLNT_S_BUFFER_EMPTY

	opGetActivateResult  = 3
	clientInitialize     = 3
	clientStart          = 10
	clientStop           = 11
	clientSetEventHandle = 13
	clientGetService     = 14
	captureGetBuffer     = 3
	captureReleaseBuffer = 4
)

var (
	mmdevapi                    = windows.NewLazySystemDLL("mmdevapi.dll")
	activateAudioInterfaceAsync = mmdevapi.NewProc("ActivateAudioInterfaceAsync")
	findWindow                  = user32.NewProc("FindWindowW")
	kernel32                    = windows.NewLazySystemDLL("kernel32.dll")
	queryPerformanceCounter     = kernel32.NewProc("QueryPerformanceCounter")
	queryPerformanceFrequency   = kernel32.NewProc("QueryPerformanceFrequency")

	iidUnknown            = mustGUID("{00000000-0000-0000-C000-000000000046}")
	iidAgileObject        = mustGUID("{94EA2B94-E9CC-49E0-C0FF-EE64CA8F5B90}")
	iidCompletionHandler  = mustGUID("{41D949AB-9862-444A-80F6-C261334DA5EB}")
	iidAudioClient        = mustGUID("{1CB9AD4C-DBFA-4C32-B178-C2F568A703B2}")
	iidAudioCaptureClient = mustGUID("{C8ADBD64-E71E-48A0-A4DE-185C395CD317}")
)

func mustGUID(s string) windows.GUID {
	g, err := windows.GUIDFromString(s)
	if err != nil {
		panic(err)
	}
	return g
}

// now100ns reads the performance counter in the 100 ns units of packet times.
func now100ns() uint64 {
	var count, freq int64
	queryPerformanceCounter.Call(uintptr(unsafe.Pointer(&count)))
	queryPerformanceFrequency.Call(uintptr(unsafe.Pointer(&freq)))
	return uint64(count/freq*10_000_000 + count%freq*10_000_000/freq)
}

// comCall calls method index of a COM object.
func comCall(obj unsafe.Pointer, method int, args ...uintptr) uintptr {
	vtbl := *(**[16]uintptr)(obj)
	hr, _, _ := syscall.SyscallN(vtbl[method], append([]uintptr{uintptr(obj)}, args...)...)
	return hr
}

func comRelease(obj unsafe.Pointer) {
	if obj != nil {
		comCall(obj, 2)
	}
}

func failed(hr uintptr) bool { return int32(hr) < 0 }

func hresultError(what string, hr uintptr) error {
	return fmt.Errorf("%s: HRESULT 0x%08X", what, uint32(hr))
}
