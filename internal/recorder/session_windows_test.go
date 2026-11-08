package recorder

import (
	"bytes"
	"context"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	framecapture "github.com/soocke/pixel-bot-go/domain/capture"
	"github.com/soocke/pixel-bot-go/internal/dataset"
)

type limitedFrames struct {
	left   int
	cancel context.CancelFunc
}

func (f *limitedFrames) Next() (framecapture.Frame, error) {
	f.left--
	if f.left == 0 {
		f.cancel()
	}
	// A minute apart, so the recording keeps every frame whole.
	return framecapture.Frame{Image: image.NewRGBA(image.Rect(0, 0, 2, 2)), CapturedAt: time.Now().Add(time.Duration(-f.left) * time.Minute)}, nil
}

func TestFramesOnlySessionWritesNoInputTrace(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "session")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var status bytes.Buffer

	err := Session{Dir: dir, Frames: &limitedFrames{left: 2, cancel: cancel}, Status: &status}.Run(ctx)

	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status.String(), "Saved 2 frames") {
		t.Errorf("status = %q", status.String())
	}
	if _, err := os.Stat(dataset.InputPath(dir)); !os.IsNotExist(err) {
		t.Errorf("input trace exists: %v", err)
	}
}

func TestClientOriginOfMissingWindowFails(t *testing.T) {
	if _, err := (windowTarget{handle: 1}).clientOrigin(); err == nil {
		t.Error("want an error")
	}
}

func TestIsMinimized(t *testing.T) {
	for origin, want := range map[point]bool{
		{-32000, -32000}: true,
		{-1920, 148}:     false, // a window on a monitor left of the primary
		{0, 0}:           false,
	} {
		if got := isMinimized(origin); got != want {
			t.Errorf("isMinimized(%v) = %v", origin, got)
		}
	}
}

func TestSessionWithUnknownWindowCreatesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "session")
	err := Session{Dir: dir, Window: "no window has this title 7f3a", Status: &bytes.Buffer{}}.Run(context.Background())
	if err == nil {
		t.Fatal("want an error")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("directory exists: %v", err)
	}
}
