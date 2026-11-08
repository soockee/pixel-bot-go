package dataset

import (
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/domain/capture"
	"github.com/soocke/pixel-bot-go/domain/detect"
)

type countingFrames struct {
	limit  int
	calls  int
	cancel context.CancelFunc
}

func (f *countingFrames) Next() (capture.Frame, error) {
	f.calls++
	if f.calls == f.limit {
		f.cancel()
	}
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255 // frames are opaque
	}
	img.Pix[0] = byte(f.calls)
	// fullEvery apart, so every frame is a whole one.
	return capture.Frame{Image: img, CapturedAt: time.Unix(0, 0).Add(time.Duration(f.calls) * fullEvery)}, nil
}

func writeLabels(t *testing.T, dir, labels string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, labelsFile), []byte(labels), 0o644); err != nil {
		t.Fatal(err)
	}
}

// slowStop cancels during its first frame and then outlasts a tick, so the
// cancelled context and the ticker are both ready when Record loops.
type slowStop struct{ countingFrames }

func (f *slowStop) Next() (capture.Frame, error) {
	frame, err := f.countingFrames.Next()
	time.Sleep(recordInterval + recordInterval/2)
	return frame, err
}

func TestRecordGrabsNoFrameAfterStop(t *testing.T) {
	// select picks at random when both are ready, so a single run could pass by luck.
	for range 12 {
		ctx, cancel := context.WithCancel(context.Background())
		dir := filepath.Join(t.TempDir(), "session")
		if err := Create(dir); err != nil {
			t.Fatal(err)
		}
		frames := &slowStop{countingFrames{limit: 1, cancel: cancel}}
		if _, err := Record(ctx, frames, detect.NewMarkerLocator(), dir, time.Unix(0, 0)); err != nil {
			t.Fatal(err)
		}
		cancel()
		if frames.calls != 1 {
			t.Fatalf("Record grabbed %d frames, want 1: it kept going after the stop", frames.calls)
		}
	}
}

func TestRecordThenLoad(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "session")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := Create(dir); err != nil {
		t.Fatal(err)
	}
	n, err := Record(ctx, &countingFrames{limit: 3, cancel: cancel}, detect.NewMarkerLocator(), dir, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("wrote %d frames, want 3", n)
	}
	writeLabels(t, dir, `{"casts":[{"start_ms":0,"end_ms":100000,"bobber":{"x":1,"y":2,"w":3,"h":4}}]}`)

	d, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "session" || len(d.Frames) != 3 {
		t.Fatalf("got %s with %d frames", d.Name, len(d.Frames))
	}
	for i, f := range d.Frames {
		img, err := f.Load()
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds().Dx() != 4 || img.Pix[0] != byte(i+1) {
			t.Errorf("frame %d: size %v, first byte %d", i, img.Bounds(), img.Pix[0])
		}
	}
}

// scriptedFrames plays images captured at the given milliseconds.
type scriptedFrames struct {
	images []*image.RGBA
	ms     []int64
	calls  int
	cancel context.CancelFunc
}

func (f *scriptedFrames) Next() (capture.Frame, error) {
	i := f.calls
	f.calls++
	if f.calls == len(f.images) {
		f.cancel()
	}
	return capture.Frame{Image: f.images[i], CapturedAt: time.UnixMilli(f.ms[i])}, nil
}

func TestRecordKeepsOnlyTheBobberBetweenWholeFrames(t *testing.T) {
	scene := func(marker bool) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, 800, 600))
		for i := range img.Pix {
			img.Pix[i] = 90 // grey, so black shows what was dropped
		}
		if marker {
			detect.DrawMarker(img, image.Rect(500, 300, 560, 360))
		}
		return img
	}
	// A whole frame, one with the bobber, one without, and fullEvery later a whole one.
	images := []*image.RGBA{scene(true), scene(true), scene(false), scene(false)}
	last := 100 + fullEvery.Milliseconds()
	ms := []int64{100, 200, 300, last}

	dir := filepath.Join(t.TempDir(), "session")
	if err := Create(dir); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n, err := Record(ctx, &scriptedFrames{images: images, ms: ms, cancel: cancel}, detect.NewMarkerLocator(), dir, time.UnixMilli(0))
	if err != nil {
		t.Fatal(err)
	}
	writeLabels(t, dir, `{"casts":[]}`)
	d, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 || len(d.Frames) != 3 || d.Frames[0].MS != 100 || d.Frames[1].MS != 200 || d.Frames[2].MS != last {
		t.Fatalf("wrote %d frames: %v; want the whole one at 100, the bobber at 200, the whole one at %d", n, d.Frames, last)
	}
	bobber, err := d.Frames[1].Load()
	if err != nil {
		t.Fatal(err)
	}
	if bobber.Bounds() != images[1].Bounds() {
		t.Fatalf("bobber frame is %v, want the window's %v", bobber.Bounds(), images[1].Bounds())
	}
	if got := bobber.RGBAAt(530, 330); got != images[1].RGBAAt(530, 330) {
		t.Errorf("inside the bobber square: %v, want the scene", got)
	}
	if got := bobber.RGBAAt(10, 10); got.R != 0 || got.A != 0 {
		t.Errorf("far from the bobber: %v, want black", got)
	}
	if _, found := detect.NewMarkerLocator().Locate(bobber); !found {
		t.Error("the marker must stay findable in a bobber frame")
	}
}

func TestCreateRefusesAnExistingDirectory(t *testing.T) {
	if err := Create(t.TempDir()); err == nil {
		t.Error("want an error")
	}
}

type brokenFrames struct{}

func (brokenFrames) Next() (capture.Frame, error) { return capture.Frame{}, errors.New("no screen") }

func TestRecordStopsOnCaptureFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "session")
	if err := Create(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := Record(context.Background(), brokenFrames{}, detect.NewMarkerLocator(), dir, time.Now()); err == nil {
		t.Error("want an error")
	}
}

func TestPartialFramesAreSkipped(t *testing.T) {
	dir := t.TempDir()
	frames := FramesPath(dir)
	if err := os.Mkdir(frames, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"000000100.png", "000000200.png.tmp"} {
		if err := os.WriteFile(filepath.Join(frames, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeLabels(t, dir, `{}`)

	d, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Frames) != 1 || d.Frames[0].MS != 100 {
		t.Errorf("frames = %+v", d.Frames)
	}
	if got := filepath.Base(LatestFrame(frames)); got != "000000100.png" {
		t.Errorf("LatestFrame = %q", got)
	}
}

func TestLoadRejectsBadLabels(t *testing.T) {
	tests := []struct{ name, labels, want string }{
		{"not json", `{`, "labels.json"},
		{"reversed cast", `{"casts":[{"start_ms":5,"end_ms":1,"bobber":{"w":1,"h":1}}]}`, "cast 0"},
		{"empty bobber", `{"casts":[{"start_ms":1,"end_ms":5}]}`, "bobber"},
		{"bite outside cast", `{"casts":[{"start_ms":1,"end_ms":5,"bobber":{"w":1,"h":1},"bite_ms":9}]}`, "bite_ms"},
		{"reversed empty span", `{"empty":[{"start_ms":5,"end_ms":1}]}`, "empty span"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeLabels(t, dir, tt.labels)
			if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestFramesInIncludesBothEnds(t *testing.T) {
	d := &Dataset{Frames: []FrameRef{{MS: 0}, {MS: 100}, {MS: 200}, {MS: 300}}}
	tests := []struct {
		span Span
		want int
	}{
		{Span{100, 200}, 2},
		{Span{50, 250}, 2},
		{Span{0, 300}, 4},
		{Span{400, 500}, 0},
	}
	for _, tt := range tests {
		if got := len(d.FramesIn(tt.span)); got != tt.want {
			t.Errorf("FramesIn(%v) = %d frames, want %d", tt.span, got, tt.want)
		}
	}
}
