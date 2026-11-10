package capture

// Windows Graphics Capture of one window: the game hands over every frame it
// draws, also while other windows cover it, and a grab copies only the asked
// area out of the latest one on the GPU. That keeps a background run's bite
// watch at the game's frame rate, where PrintWindow manages about 12 fps.
// COM and WinRT are called through their vtables; there is no cgo.

import (
	"errors"
	"fmt"
	"image"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	d3d11                     = windows.NewLazySystemDLL("d3d11.dll")
	combase                   = windows.NewLazySystemDLL("combase.dll")
	procDwmAttribute          = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmGetWindowAttribute")
	procD3D11CreateDevice     = d3d11.NewProc("D3D11CreateDevice")
	procCreateWinRTD3DDevice  = d3d11.NewProc("CreateDirect3D11DeviceFromDXGIDevice")
	procRoInitialize          = combase.NewProc("RoInitialize")
	procRoGetActivationFactor = combase.NewProc("RoGetActivationFactory")
	procWindowsCreateString   = combase.NewProc("WindowsCreateString")
	procWindowsDeleteString   = combase.NewProc("WindowsDeleteString")
)

var (
	iidDXGIDevice           = mustGUID("{54ec77fa-1377-44e6-8c32-88fd5f44c84c}")
	iidWinRTD3DDevice       = mustGUID("{a37624ab-8d5f-4650-9d3e-9eae3d9bc670}")
	iidCaptureItemInterop   = mustGUID("{3628e81b-3cac-4c60-b7f4-23ce0e0c3356}")
	iidCaptureItem          = mustGUID("{79c3f95b-31f7-4ec2-a464-632ef5d30760}")
	iidFramePoolStatics2    = mustGUID("{589b103f-6bbc-5df5-a991-02e28b3b66d5}")
	iidCaptureSession2      = mustGUID("{2c39ae40-7d2e-5044-804e-8b6799d4cf9e}")
	iidCaptureSession3      = mustGUID("{f2cdd966-22ae-5ea1-9596-3a289344c3be}")
	iidClosable             = mustGUID("{30d5a829-7fa4-4026-83bb-d75bae4ea99e}")
	iidDxgiInterfaceAccess  = mustGUID("{a9b3d012-3df2-4ee3-b8d1-8695f457d3c1}")
	iidD3D11Texture2D       = mustGUID("{6f15aaf2-d208-4e89-9ab4-489535d34f9c}")
	errNoFrame              = fmt.Errorf("%w: the game has drawn no frame yet", ErrFrameUnavailable)
	errStaleFrame           = fmt.Errorf("%w: the game has drawn no new frame for a second", ErrFrameUnavailable)
	staleAfter              = time.Second
	firstFrameWait          = 500 * time.Millisecond
	formatB8G8R8A8          = uintptr(87) // DXGI_FORMAT_B8G8R8A8_UNORM, also the WinRT DirectXPixelFormat value
	framePoolBuffers        = uintptr(2)
	dwmwaExtendedFrameBound = uintptr(9)
)

// vtable slots used below, counted from the start of each interface's vtable.
const (
	slotQueryInterface = 0
	slotRelease        = 2
	// IInspectable adds three slots, so WinRT methods start at 6.
	slotWinRT = 6
	// IGraphicsCaptureItem
	slotItemSize = 7
	// IDirect3D11CaptureFramePool
	slotPoolRecreate      = 6
	slotPoolTryGetNext    = 7
	slotPoolCreateSession = 10
	// IDirect3D11CaptureFrame
	slotFrameSurface     = 6
	slotFrameTime        = 7
	slotFrameContentSize = 8
	// IGraphicsCaptureSession2 and 3 setters
	slotSessionPut = 7
	// ID3D11Device, ID3D11DeviceContext
	slotCreateTexture2D        = 5
	slotMap                    = 14
	slotUnmap                  = 15
	slotCopySubresourceRegion  = 46
	slotInterfaceAccessGetIntf = 3
)

// WindowStream captures one window with Windows Graphics Capture. Grab is
// safe for concurrent use. Close it when the run ends; until then Windows may
// draw a capture border around the window.
type WindowStream struct {
	mu      sync.Mutex
	window  WindowIdentity
	device  unsafe.Pointer // ID3D11Device
	context unsafe.Pointer // ID3D11DeviceContext
	winrt   unsafe.Pointer // IDirect3DDevice
	item    unsafe.Pointer // IGraphicsCaptureItem
	pool    unsafe.Pointer // IDirect3D11CaptureFramePool
	session unsafe.Pointer // IGraphicsCaptureSession
	size    image.Point    // the frame pool's buffer size

	frame   unsafe.Pointer // the latest IDirect3D11CaptureFrame, held until a newer one comes
	texture unsafe.Pointer // its ID3D11Texture2D
	frameAt time.Time
	content image.Point // its content size

	restartedAt time.Time // when the session was last restarted for going quiet

	staging     unsafe.Pointer // CPU-readable ID3D11Texture2D
	stagingSize image.Point
	mta         chan struct{} // closed by Close; keeps the process's MTA alive
}

// OpenWindowStream starts capturing w. It fails on Windows before 10 1903 or
// without a hardware Direct3D 11 device.
func OpenWindowStream(w WindowIdentity) (_ *WindowStream, err error) {
	s := &WindowStream{window: w, mta: make(chan struct{})}
	defer func() {
		if err != nil {
			s.Close()
		}
	}()
	// A thread in the multithreaded apartment for the stream's lifetime makes
	// every goroutine's thread an implicit MTA member, so any of them may call
	// the free-threaded frame pool.
	ready := make(chan struct{})
	go func() {
		// Never unlocked: the thread ends with the goroutine, so it never
		// returns to Go with COM still started on it.
		runtime.LockOSThread()
		procRoInitialize.Call(1) // RO_INIT_MULTITHREADED; an earlier MTA init is fine too
		close(ready)
		<-s.mta
	}()
	<-ready

	const hardware, bgraSupport, sdkVersion = 1, 0x20, 7
	if hr, _, _ := procD3D11CreateDevice.Call(0, hardware, 0, bgraSupport, 0, 0, sdkVersion,
		uintptr(unsafe.Pointer(&s.device)), 0, uintptr(unsafe.Pointer(&s.context))); failed(hr) {
		return nil, hresultError("capture: D3D11CreateDevice", hr)
	}
	dxgi, err := query(s.device, &iidDXGIDevice)
	if err != nil {
		return nil, err
	}
	var inspectable unsafe.Pointer
	hr, _, _ := procCreateWinRTD3DDevice.Call(uintptr(dxgi), uintptr(unsafe.Pointer(&inspectable)))
	comRelease(dxgi)
	if failed(hr) {
		return nil, hresultError("capture: CreateDirect3D11DeviceFromDXGIDevice", hr)
	}
	s.winrt, err = query(inspectable, &iidWinRTD3DDevice)
	comRelease(inspectable)
	if err != nil {
		return nil, err
	}

	interop, err := factory("Windows.Graphics.Capture.GraphicsCaptureItem", &iidCaptureItemInterop)
	if err != nil {
		return nil, err
	}
	const slotCreateForWindow = 3
	hr = comCall(interop, slotCreateForWindow, uintptr(w.Handle), uintptr(unsafe.Pointer(&iidCaptureItem)), uintptr(unsafe.Pointer(&s.item)))
	comRelease(interop)
	if failed(hr) {
		return nil, hresultError("capture: CreateForWindow", hr)
	}
	var size [2]int32
	if hr := comCall(s.item, slotItemSize, uintptr(unsafe.Pointer(&size))); failed(hr) {
		return nil, hresultError("capture: GraphicsCaptureItem.Size", hr)
	}
	s.size = image.Pt(int(size[0]), int(size[1]))
	if err := s.startSession(); err != nil {
		return nil, err
	}
	return s, nil
}

// startSession creates the frame pool and the capture session and starts it.
func (s *WindowStream) startSession() error {
	statics, err := factory("Windows.Graphics.Capture.Direct3D11CaptureFramePool", &iidFramePoolStatics2)
	if err != nil {
		return err
	}
	hr := comCall(statics, slotWinRT, uintptr(s.winrt), formatB8G8R8A8, framePoolBuffers, packSize(s.size), uintptr(unsafe.Pointer(&s.pool)))
	comRelease(statics)
	if failed(hr) {
		return hresultError("capture: CreateFreeThreaded", hr)
	}
	if hr := comCall(s.pool, slotPoolCreateSession, uintptr(s.item), uintptr(unsafe.Pointer(&s.session))); failed(hr) {
		return hresultError("capture: CreateCaptureSession", hr)
	}
	// The pointer must not hide the bobber, and the yellow border is only
	// optional on Windows 11; older versions keep it.
	if s2, err := query(s.session, &iidCaptureSession2); err == nil {
		comCall(s2, slotSessionPut, 0) // IsCursorCaptureEnabled = false
		comRelease(s2)
	}
	if s3, err := query(s.session, &iidCaptureSession3); err == nil {
		comCall(s3, slotSessionPut, 0) // IsBorderRequired = false
		comRelease(s3)
	}
	if hr := comCall(s.session, slotWinRT); failed(hr) { // StartCapture
		return hresultError("capture: StartCapture", hr)
	}
	return nil
}

// Grab copies area, in screen coordinates, out of the window's latest frame.
// It fails when the game has drawn nothing new for a second, for example
// while it is minimized, so a frozen picture never passes for a live one.
func (s *WindowStream) Grab(area image.Rectangle) (Frame, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pool == nil {
		return Frame{}, errors.New("capture: the window stream is closed")
	}
	if err := s.latest(); err != nil {
		return Frame{}, err
	}
	client, err := s.window.Area()
	if err != nil {
		return Frame{}, err
	}
	origin, err := s.frameOrigin()
	if err != nil {
		return Frame{}, err
	}
	r := area.Intersect(client).Intersect(image.Rectangle{Max: s.content}.Add(origin))
	if r.Empty() {
		return Frame{}, fmt.Errorf("capture: area %v is outside the window %v", area, client)
	}
	img, err := s.copyOut(r.Sub(origin))
	if err != nil {
		return Frame{}, err
	}
	return Frame{Image: img, Origin: r.Min, CapturedAt: s.frameAt}, nil
}

// latest takes the newest frame the pool holds, waiting a moment for the
// first one, and releases the frame it replaces.
func (s *WindowStream) latest() error {
	deadline := time.Now()
	if s.frame == nil {
		deadline = deadline.Add(firstFrameWait)
	}
	for {
		var newest unsafe.Pointer
		for {
			var f unsafe.Pointer
			if hr := comCall(s.pool, slotPoolTryGetNext, uintptr(unsafe.Pointer(&f))); failed(hr) {
				return hresultError("capture: TryGetNextFrame", hr)
			}
			if f == nil {
				break
			}
			closeFrame(newest)
			newest = f
		}
		if newest != nil {
			return s.hold(newest)
		}
		stale := s.frame != nil && time.Since(s.frameAt) > staleAfter
		if s.frame != nil && !stale {
			return nil
		}
		// A session can stop delivering while the game still draws (seen right
		// after a reel), and a new one gets frames at once.
		if stale && time.Since(s.restartedAt) > staleAfter {
			if err := s.restart(); err != nil {
				return err
			}
			deadline = time.Now().Add(firstFrameWait)
			continue
		}
		if time.Now().After(deadline) {
			if stale {
				return errStaleFrame
			}
			return errNoFrame
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// restart replaces the frame pool and the session. The current frame stays
// until a new one comes.
func (s *WindowStream) restart() error {
	s.restartedAt = time.Now()
	closeFrame(s.session)
	closeFrame(s.pool)
	s.session, s.pool = nil, nil
	return s.startSession()
}

// hold makes f the current frame. A frame of a resized window comes from
// buffers of the old size, so the pool is recreated and f still serves.
func (s *WindowStream) hold(f unsafe.Pointer) error {
	var size [2]int32
	var ticks int64
	comCall(f, slotFrameContentSize, uintptr(unsafe.Pointer(&size)))
	comCall(f, slotFrameTime, uintptr(unsafe.Pointer(&ticks)))
	surface, err := getOut(f, slotFrameSurface)
	if err != nil {
		closeFrame(f)
		return err
	}
	access, err := query(surface, &iidDxgiInterfaceAccess)
	comRelease(surface)
	if err != nil {
		closeFrame(f)
		return err
	}
	var texture unsafe.Pointer
	hr := comCall(access, slotInterfaceAccessGetIntf, uintptr(unsafe.Pointer(&iidD3D11Texture2D)), uintptr(unsafe.Pointer(&texture)))
	comRelease(access)
	if failed(hr) {
		closeFrame(f)
		return hresultError("capture: IDirect3DDxgiInterfaceAccess.GetInterface", hr)
	}
	comRelease(s.texture)
	closeFrame(s.frame)
	s.frame, s.texture = f, texture
	s.frameAt = time.Now().Add(-time.Duration(int64(now100ns())-ticks) * 100) // SystemRelativeTime is QPC time in 100 ns
	// A new session's first frame keeps the time the window last changed, but
	// it shows the window as it is now.
	if s.frameAt.Before(s.restartedAt) {
		s.frameAt = s.restartedAt
	}
	got := image.Pt(int(size[0]), int(size[1]))
	s.content = image.Rectangle{Max: got}.Intersect(image.Rectangle{Max: s.size}).Size()
	if got != s.size && got.X > 0 && got.Y > 0 {
		s.size = got
		comCall(s.pool, slotPoolRecreate, uintptr(s.winrt), formatB8G8R8A8, framePoolBuffers, packSize(got))
	}
	return nil
}

// frameOrigin is the screen position of the frame's top-left pixel: a window
// capture covers the window's visible frame, of which the client is a part.
func (s *WindowStream) frameOrigin() (image.Point, error) {
	defer physicalPixels()()
	var r struct{ Left, Top, Right, Bottom int32 }
	if hr, _, _ := procDwmAttribute.Call(uintptr(s.window.Handle), dwmwaExtendedFrameBound, uintptr(unsafe.Pointer(&r)), unsafe.Sizeof(r)); failed(hr) {
		return image.Point{}, hresultError("capture: DwmGetWindowAttribute", hr)
	}
	return image.Pt(int(r.Left), int(r.Top)), nil
}

// copyOut copies r, in frame pixels, into the staging texture on the GPU and
// reads it back as RGBA.
func (s *WindowStream) copyOut(r image.Rectangle) (*image.RGBA, error) {
	if s.staging == nil || s.stagingSize != s.size {
		comRelease(s.staging)
		s.staging = nil
		desc := struct {
			Width, Height, MipLevels, ArraySize, Format uint32
			SampleCount, SampleQuality                  uint32
			Usage, BindFlags, CPUAccessFlags, MiscFlags uint32
		}{uint32(s.size.X), uint32(s.size.Y), 1, 1, uint32(formatB8G8R8A8), 1, 0, 3, 0, 0x20000, 0} // D3D11_USAGE_STAGING, D3D11_CPU_ACCESS_READ
		if hr := comCall(s.device, slotCreateTexture2D, uintptr(unsafe.Pointer(&desc)), 0, uintptr(unsafe.Pointer(&s.staging))); failed(hr) {
			return nil, hresultError("capture: CreateTexture2D", hr)
		}
		s.stagingSize = s.size
	}
	box := struct{ Left, Top, Front, Right, Bottom, Back uint32 }{
		uint32(r.Min.X), uint32(r.Min.Y), 0, uint32(r.Max.X), uint32(r.Max.Y), 1,
	}
	comCall(s.context, slotCopySubresourceRegion, uintptr(s.staging), 0, 0, 0, 0, uintptr(s.texture), 0, uintptr(unsafe.Pointer(&box)))
	var mapped struct {
		Data                 unsafe.Pointer
		RowPitch, DepthPitch uint32
	}
	const mapRead = 1
	if hr := comCall(s.context, slotMap, uintptr(s.staging), 0, mapRead, 0, uintptr(unsafe.Pointer(&mapped))); failed(hr) {
		return nil, hresultError("capture: Map", hr)
	}
	defer comCall(s.context, slotUnmap, uintptr(s.staging), 0)
	w, h := r.Dx(), r.Dy()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		row := unsafe.Slice((*byte)(unsafe.Add(mapped.Data, y*int(mapped.RowPitch))), w*4)
		out := img.Pix[y*img.Stride:][:w*4]
		for i := 0; i < len(row); i += 4 {
			out[i], out[i+1], out[i+2], out[i+3] = row[i+2], row[i+1], row[i], 0xFF
		}
	}
	return img, nil
}

// Close stops the capture and releases everything it holds.
func (s *WindowStream) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	comRelease(s.staging)
	comRelease(s.texture)
	closeFrame(s.frame)
	closeFrame(s.session)
	closeFrame(s.pool)
	for _, p := range []unsafe.Pointer{s.item, s.winrt, s.context, s.device} {
		comRelease(p)
	}
	s.staging, s.texture, s.frame, s.session, s.pool = nil, nil, nil, nil, nil
	s.item, s.winrt, s.context, s.device = nil, nil, nil, nil
	select {
	case <-s.mta:
	default:
		close(s.mta)
	}
}

func getOut(obj unsafe.Pointer, slot int) (unsafe.Pointer, error) {
	var out unsafe.Pointer
	if hr := comCall(obj, slot, uintptr(unsafe.Pointer(&out))); failed(hr) {
		return nil, hresultError("capture: "+fmt.Sprintf("vtable slot %d", slot), hr)
	}
	return out, nil
}

func query(obj unsafe.Pointer, iid *windows.GUID) (unsafe.Pointer, error) {
	var out unsafe.Pointer
	if hr := comCall(obj, slotQueryInterface, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out))); failed(hr) {
		return nil, hresultError("capture: QueryInterface "+iid.String(), hr)
	}
	return out, nil
}

// closeFrame closes a WinRT object (frame, session or pool) and releases it.
func closeFrame(obj unsafe.Pointer) {
	if obj == nil {
		return
	}
	if c, err := query(obj, &iidClosable); err == nil {
		comCall(c, slotWinRT)
		comRelease(c)
	}
	comRelease(obj)
}

func factory(class string, iid *windows.GUID) (unsafe.Pointer, error) {
	name, err := windows.UTF16FromString(class)
	if err != nil {
		return nil, err
	}
	var hstring uintptr
	if hr, _, _ := procWindowsCreateString.Call(uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)-1), uintptr(unsafe.Pointer(&hstring))); failed(hr) {
		return nil, hresultError("capture: WindowsCreateString", hr)
	}
	defer procWindowsDeleteString.Call(hstring)
	var f unsafe.Pointer
	if hr, _, _ := procRoGetActivationFactor.Call(hstring, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&f))); failed(hr) {
		return nil, hresultError("capture: RoGetActivationFactory "+class, hr)
	}
	return f, nil
}

// packSize passes a SizeInt32 by value: two int32 in one register on amd64.
func packSize(p image.Point) uintptr { return uintptr(uint32(p.X)) | uintptr(uint32(p.Y))<<32 }
