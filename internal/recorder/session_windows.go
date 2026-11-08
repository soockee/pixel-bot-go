// Package recorder records a play session into one dataset directory: screen
// frames plus, when a window is named, the user's own mouse and key input in
// that window and the sound its process plays. It only observes; it never sends
// input, and the game stays audible.
package recorder

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	framecapture "github.com/soocke/pixel-bot-go/domain/capture"
	"github.com/soocke/pixel-bot-go/domain/detect"
	"github.com/soocke/pixel-bot-go/internal/dataset"
	"github.com/soocke/pixel-bot-go/internal/playtrace"
)

// Session describes one recording.
type Session struct {
	Dir    string              // new dataset directory
	Frames dataset.FrameSource // screen capture
	Window string              // exact title of the window to record input and sound from; empty records frames only
	Live   bool                // flush the input trace often, so a viewer can follow it
	Label  string              // shared with other traces from this playtest
	Keys   bool                // record keys as well as the mouse
	Status io.Writer           // progress messages
}

// Run records until ctx ends or either recording fails. Frames and input share
// one start time, so their timestamps line up.
func (s Session) Run(parent context.Context) error {
	var target windowTarget
	if s.Window != "" {
		var err error
		if target, err = findTarget(s.Window); err != nil {
			return err
		}
	}
	var info dataset.SessionInfo
	if s.Window != "" {
		var err error
		if info, err = s.sessionInfo(target); err != nil {
			return err
		}
	}
	if err := dataset.Create(s.Dir); err != nil {
		return err
	}
	if s.Window != "" {
		if err := dataset.WriteSessionInfo(s.Dir, info); err != nil {
			return err
		}
	}

	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	start := time.Now()
	var frames int
	var frameErr, inputErr, audioErr error
	var wg sync.WaitGroup
	wg.Go(func() {
		defer cancel()
		frames, frameErr = dataset.Record(ctx, s.Frames, detect.NewMarkerLocator(), s.Dir, start)
	})
	if s.Window != "" {
		wg.Go(func() {
			defer cancel()
			inputErr = run(ctx, target, dataset.InputPath(s.Dir), s.Status, s.Live, s.Keys, s.Label, start)
		})
		wg.Go(func() {
			// Frames and input matter more than sound, so a sound failure does not stop them.
			if err := recordAudio(ctx, target.pid, s.Dir, start); err != nil {
				fmt.Fprintf(s.Status, "Recording without sound: %v\n", err)
				audioErr = fmt.Errorf("frames and input were saved, but not the sound: %w", err)
			}
		})
	}
	wg.Wait()
	fmt.Fprintf(s.Status, "Saved %d frames to %s.\n", frames, s.Dir)
	return errors.Join(frameErr, inputErr, audioErr)
}

// recordAudio saves the sound of process pid until ctx ends.
func recordAudio(ctx context.Context, pid uint32, dir string, start time.Time) (err error) {
	w, err := dataset.CreateAudio(dir, start)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, w.Close()) }()
	return framecapture.CaptureAudio(ctx, pid, w.Write)
}

// sessionInfo works out how to turn a position in the window into a frame pixel.
func (s Session) sessionInfo(target windowTarget) (dataset.SessionInfo, error) {
	frame, err := s.Frames.Next()
	if err != nil {
		return dataset.SessionInfo{}, err
	}
	origin, err := target.clientOrigin()
	if err != nil {
		return dataset.SessionInfo{}, err
	}
	return dataset.SessionInfo{ClientToFrame: origin.Sub(frame.Origin)}, nil
}

func run(parent context.Context, target windowTarget, out string, status io.Writer, live, recordKeys bool, session string, start time.Time) (err error) {
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
	if live {
		if err := traceWriter.Flush(); err != nil {
			return err
		}
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
