package desktop

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

func TestMoveOverlay(t *testing.T) {
	var calls []bool
	running := false
	var failure error
	loop := NewLoop(func() {})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx, time.Hour)
	cmds := Commands{
		Running:     func() bool { return running },
		MoveOverlay: func(on bool) error { calls = append(calls, on); return failure },
	}
	s := NewService(loop, NewView(NewFrames(), func(State) {}), cmds, nil, config.DefaultConfig(),
		filepath.Join(t.TempDir(), "config.json"), slog.New(slog.DiscardHandler))

	if err := s.MoveOverlay(true); err != nil {
		t.Fatal(err)
	}
	if err := s.MoveOverlay(false); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(calls, []bool{true, false}) {
		t.Fatalf("commands = %v, want a start and an end", calls)
	}

	// A run owns the pointer, so placing the overlay waits; ending a move is always allowed.
	running = true
	if err := s.MoveOverlay(true); err == nil {
		t.Fatal("started placing the overlay during a run")
	}
	if err := s.MoveOverlay(false); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(calls, []bool{true, false, false}) {
		t.Fatalf("commands = %v: the refused start must not reach the app", calls)
	}

	running, failure = false, errors.New("start the game first")
	if err := s.MoveOverlay(true); err == nil || err.Error() != "start the game first" {
		t.Fatalf("the app's reason was lost: %v", err)
	}
}

func TestViewSendsTheOverlayPlace(t *testing.T) {
	var sent []State
	v := NewView(NewFrames(), func(s State) { sent = append(sent, s) })
	v.Flush()
	v.SetOverlay(0.25, 0.8, true)
	v.Flush()
	v.Flush()
	last := sent[len(sent)-1]
	if len(sent) != 2 || last.OverlayX != 0.25 || last.OverlayY != 0.8 || !last.OverlayMove {
		t.Fatalf("sent %d states, last overlay = %v, %v, moving %v", len(sent), last.OverlayX, last.OverlayY, last.OverlayMove)
	}
}
