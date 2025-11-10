//go:build windows

package debug

// Memory/RSS periodic logger enabled when config.Debug is true.
// Logs working set (RSS) along with Go heap stats to correlate native vs heap growth.

import (
	"log/slog"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// processMemoryCounters matches PROCESS_MEMORY_COUNTERS from psapi.
type processMemoryCounters struct {
	cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

var (
	modPsapi                 = windows.NewLazySystemDLL("psapi.dll")
	procGetProcessMemoryInfo = modPsapi.NewProc("GetProcessMemoryInfo")
)

// startMemLogger launches a goroutine that logs memory stats every interval.
// It is best-effort; failures to query RSS are logged once and suppressed.
func StartMemLogger(interval time.Duration, logger *slog.Logger) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		var rssErrLogged bool
		for range ticker.C {
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			gcount := runtime.NumGoroutine()
			rss := uint64(0)
			pmc := processMemoryCounters{cb: uint32(unsafe.Sizeof(processMemoryCounters{}))}
			r1, _, err := procGetProcessMemoryInfo.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&pmc)), uintptr(pmc.cb))
			if r1 != 0 {
				rss = uint64(pmc.WorkingSetSize)
			} else if !rssErrLogged {
				logger.Warn("memlog: GetProcessMemoryInfo call failed", slog.String("err", err.Error()))
				rssErrLogged = true
			}
			logger.Info("memstats",
				slog.Int("goroutines", gcount),
				slog.Uint64("heap_alloc", ms.HeapAlloc),
				slog.Uint64("heap_inuse", ms.HeapInuse),
				slog.Uint64("heap_idle", ms.HeapIdle),
				slog.Uint64("heap_sys", ms.HeapSys),
				slog.Uint64("next_gc", ms.NextGC),
				slog.Uint64("rss", rss),
				slog.Uint64("num_gc", uint64(ms.NumGC)),
			)
		}
	}()
}
