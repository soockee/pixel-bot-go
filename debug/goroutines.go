package debug

// Debug goroutine metrics logger. Started only when config.Debug is true.
// Emits goroutine count (runtime metrics) and stack usage at a fixed interval.
// Intentionally minimal: focuses solely on ruling out goroutine / stack driven RSS growth.

import (
	"log/slog"
	"runtime"
	"runtime/metrics"
	"time"
)

// startGoroutineLogger launches a ticker that logs goroutine count and stack memory.
// It is lightweight; disable by running without the debug flag.
func StartGoroutineLogger(interval time.Duration, logger *slog.Logger) {
	if interval <= 0 {
		interval = time.Second
	}

	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		samples := []metrics.Sample{{Name: "/sched/goroutines:goroutines"}}
		for range t.C {
			metrics.Read(samples)
			goroutines := samples[0].Value.Uint64()
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			logger.Info("goroutine-stacks",
				slog.Uint64("goroutines", goroutines),
				slog.Uint64("stack_inuse", uint64(ms.StackInuse)),
				slog.Uint64("stack_sys", uint64(ms.StackSys)),
				slog.Uint64("heap_alloc", uint64(ms.HeapAlloc)),
			)
		}
	}()
}
