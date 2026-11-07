// Package recorder records a play session into one dataset directory: screen
// frames plus, when a window is named, the user's own mouse and key input in
// that window. It only observes; it never sends input.
package recorder

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/soocke/pixel-bot-go/internal/dataset"
	"github.com/soocke/pixel-bot-go/internal/playtrace"
)

// DefaultListen is the default dashboard address.
const DefaultListen = "127.0.0.1:6070"

// Session describes one recording.
type Session struct {
	Dir    string              // new dataset directory
	Frames dataset.FrameSource // screen capture
	Window string              // exact title of the window to record input from; empty records frames only
	Listen string              // loopback dashboard address; empty disables it
	Label  string              // shared with other traces from this playtest
	Keys   bool                // record keys as well as the mouse
	Status io.Writer           // progress messages
}

// Run records until ctx ends or either recording fails. Frames and input share
// one start time, so their timestamps line up.
func (s Session) Run(parent context.Context) error {
	var target windowTarget
	var listener net.Listener
	if s.Window != "" {
		var err error
		if target, err = findTarget(s.Window); err != nil {
			return err
		}
		if s.Listen != "" {
			if listener, err = listenDashboard(s.Listen); err != nil {
				return fmt.Errorf("listen for dashboard: %w", err)
			}
			defer listener.Close()
		}
	}
	if err := dataset.Create(s.Dir); err != nil {
		return err
	}
	if s.Window != "" {
		if err := s.saveInfo(target); err != nil {
			return err
		}
	}

	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	start := time.Now()
	var frames int
	var frameErr, inputErr error
	var wg sync.WaitGroup
	wg.Go(func() {
		defer cancel()
		frames, frameErr = dataset.Record(ctx, s.Frames, s.Dir, start)
	})
	if s.Window != "" {
		wg.Go(func() {
			defer cancel()
			inputErr = run(ctx, target, dataset.InputPath(s.Dir), s.Status, listener, s.Keys, s.Label, start, dataset.FramesPath(s.Dir))
		})
	}
	wg.Wait()
	fmt.Fprintf(s.Status, "Saved %d frames to %s.\n", frames, s.Dir)
	return errors.Join(frameErr, inputErr)
}

// saveInfo stores how to turn a position in the window into a frame pixel.
func (s Session) saveInfo(target windowTarget) error {
	frame, err := s.Frames.Next()
	if err != nil {
		return err
	}
	origin, err := target.clientOrigin()
	if err != nil {
		return err
	}
	return dataset.WriteSessionInfo(s.Dir, dataset.SessionInfo{ClientToFrame: origin.Sub(frame.Origin)})
}

// View serves the dashboard for a saved session directory or trace file until
// ctx ends.
func View(ctx context.Context, path, address string, status io.Writer) error {
	listener, err := listenDashboard(address)
	if err != nil {
		return fmt.Errorf("listen for dashboard: %w", err)
	}
	defer listener.Close()
	frames := ""
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		path, frames = dataset.InputPath(path), dataset.FramesPath(path)
	}
	return viewTrace(ctx, listener, path, frames, status)
}

func run(parent context.Context, target windowTarget, out string, status io.Writer, listener net.Listener, recordKeys bool, session string, start time.Time, frames string) (err error) {
	utc := start.UTC()
	traceWriter, err := playtrace.Create(out, playtrace.Header{
		Source: playtrace.Manual, Clock: playtrace.Monotonic, SessionID: session,
		StartedUTC: &utc, Keyboard: recordKeys, DragPixels: dragPixels, DragCounts: dragCounts,
	})
	if err != nil {
		return err
	}
	end := playtrace.End{Status: "complete"}
	defer func() {
		end.ElapsedUS = time.Since(start).Microseconds()
		if err != nil {
			end.Status = "incomplete"
		}
		if closeErr := traceWriter.Close(end); closeErr != nil {
			err = errors.Join(err, closeErr)
			return
		}
		fmt.Fprintf(status, "Saved %d moves, %d clicks, %d drags and %d key transitions across %d segments to %s (%s).\n",
			end.Moves, end.Clicks, end.Drags, end.KeyEvents, end.Segments, out, end.Status)
	}()

	var flush <-chan time.Time
	if listener != nil {
		if err := traceWriter.Flush(); err != nil {
			return err
		}
		trace, err := os.Open(out)
		if err != nil {
			return err
		}
		defer trace.Close()
		stopDashboard := startDashboard(listener, trace, frames, true, status)
		defer stopDashboard()
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		flush = ticker.C
	}

	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	events := make(chan inputEvent, 4096)
	finished := make(chan error, 1)
	go func() {
		defer close(events)
		finished <- capture(ctx, target, start, events, func() {
			fmt.Fprintln(status, "Recorder armed. Focus the selected window. Press Ctrl+C in this terminal to stop.")
		}, recordKeys)
	}()

	var writeErr error
collect:
	for {
		var event inputEvent
		select {
		case <-flush:
			if writeErr == nil {
				if writeErr = traceWriter.Flush(); writeErr != nil {
					cancel()
				}
			}
			continue
		case value, ok := <-events:
			if !ok {
				break collect
			}
			event = value
		}
		if writeErr != nil {
			continue
		}
		if writeErr = traceWriter.Write(event); writeErr != nil {
			cancel()
			continue
		}
		switch event.Type {
		case "move":
			end.Moves++
		case "button_down", "button_up":
			end.ButtonEvents++
		case "key_down", "key_up":
			end.KeyEvents++
		case "click":
			end.Clicks++
		case "drag":
			end.Drags++
		case "segment_start":
			end.Segments++
		}
	}
	captureErr := <-finished
	if writeErr != nil {
		return errors.Join(writeErr, captureErr)
	}
	return captureErr
}
