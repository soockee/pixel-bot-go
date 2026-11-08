package desktop

import (
	"context"
	"time"
)

// Loop runs the UI tick and every command on one goroutine, so presenters and
// View need no locks.
type Loop struct {
	tick func()
	cmds chan func()
	done chan struct{}
}

// NewLoop returns a Loop that calls tick on every interval.
func NewLoop(tick func()) *Loop {
	return &Loop{tick: tick, cmds: make(chan func()), done: make(chan struct{})}
}

// Run ticks and runs commands until ctx ends.
func (l *Loop) Run(ctx context.Context, interval time.Duration) {
	defer close(l.done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.tick()
		case cmd := <-l.cmds:
			cmd()
			l.tick() // show the result now, not on the next tick
		}
	}
}

// Do runs f on the loop and waits for it. It returns false, without running f,
// once the loop has stopped. It must not be called from the loop itself.
func (l *Loop) Do(f func()) bool {
	finished := make(chan struct{})
	select {
	case l.cmds <- func() { f(); close(finished) }:
	case <-l.done:
		return false
	}
	<-finished
	return true
}
