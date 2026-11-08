package desktop

import (
	"context"
	"image"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

func TestPickWindow(t *testing.T) {
	const wow = "World of Warcraft"
	tests := []struct {
		name    string
		titles  []string
		current string
		chosen  bool
		want    string
	}{
		{"defaults to the game", []string{"Claude", wow}, "", false, wow},
		{"falls back to the first window", []string{"Claude", "Notes"}, "", false, "Claude"},
		{"the game appears after a refresh", []string{"Claude", wow}, "Claude", false, wow},
		{"keeps a window the user chose", []string{"Claude", wow}, "Claude", true, "Claude"},
		{"keeps a chosen window that is not open", []string{wow}, "Fake Game", true, "Fake Game"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pickWindow(tt.titles, tt.current, tt.chosen); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSettingsApply(t *testing.T) {
	base := *config.DefaultConfig()
	good := settingsOf(&base)
	good.CastKey, good.ROISizePx = "r", 9999

	got, err := good.apply(base)
	if err != nil {
		t.Fatalf("valid settings: %v", err)
	}
	if got.CastKey != "r" || got.ROISizePx != 256 {
		t.Errorf("cast key %q, roi %d; want r and the clamped 256", got.CastKey, got.ROISizePx)
	}

	bad := good
	bad.StopKey, bad.Locator, bad.RecordDir = "Ctrl", "nope", " "
	if _, err := bad.apply(base); err == nil ||
		!strings.Contains(err.Error(), "Stop key, Locator, Recording folder") {
		t.Errorf("invalid settings: err = %v", err)
	}
}

func TestViewFlushSendsOnlyChanges(t *testing.T) {
	var sent []State
	v := NewView(NewFrames(), func(s State) { sent = append(sent, s) })
	v.SetStateLabel("searching")
	v.Flush()
	v.Flush()
	v.UpdateCapture(image.NewRGBA(image.Rect(0, 0, 4, 4)))
	v.Flush()
	v.PreviewReset()
	v.UpdateCapture(image.NewRGBA(image.Rect(0, 0, 4, 4)))
	v.Flush()
	if len(sent) != 3 {
		t.Fatalf("sent %d states, want 3", len(sent))
	}
	if sent[1].Frame == 0 || sent[2].Frame == sent[1].Frame {
		t.Errorf("frame versions %d then %d; a new frame needs a new version", sent[1].Frame, sent[2].Frame)
	}
}

func TestFramesServeTheNewestImage(t *testing.T) {
	f := NewFrames()
	get := func() int {
		w := httptest.NewRecorder()
		f.ServeHTTP(w, httptest.NewRequest(http.MethodGet, FramesPath+"capture", nil))
		return w.Code
	}
	if code := get(); code != http.StatusNotFound {
		t.Errorf("before a frame: %d", code)
	}
	f.Put("capture", image.NewRGBA(image.Rect(0, 0, 4, 4)))
	if code := get(); code != http.StatusOK {
		t.Errorf("after a frame: %d", code)
	}
}

func TestLoopDoRunsOnTheLoopAndStops(t *testing.T) {
	ticks := 0
	l := NewLoop(func() { ticks++ })
	ctx, cancel := context.WithCancel(context.Background())
	go l.Run(ctx, time.Hour)
	ran := false
	if !l.Do(func() { ran = true }) || !ran || ticks != 1 {
		t.Fatalf("Do: ran=%v ticks=%d", ran, ticks)
	}
	cancel()
	<-l.done
	if l.Do(func() {}) {
		t.Error("Do ran after the loop stopped")
	}
}

func TestRecordingsStayInTheFolder(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "datasets")
	frames := filepath.Join(folder, "20261108-120000", "frames")
	if err := os.MkdirAll(frames, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frames, "000000100.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewRecordings(func() string { return folder })
	list, err := r.List()
	if err != nil || len(list) != 1 || list[0].Name != "20261108-120000" {
		t.Fatalf("list = %+v, err = %v", list, err)
	}
	for path, want := range map[string]int{
		RecordingsPath + "20261108-120000": http.StatusOK,
		RecordingsPath + "..":              http.StatusNotFound,
		RecordingsPath + "../datasets":     http.StatusNotFound,
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != want {
			t.Errorf("%s: %d, want %d", path, w.Code, want)
		}
	}
	if _, err := r.Events("..", 0); err == nil {
		t.Error("events outside the folder were read")
	}
}

func TestSelectionStartsCentredWithoutASelection(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSelection(cfg, filepath.Join(t.TempDir(), "c.json"), nil)
	screen := image.Rect(-1920, 0, 0, 1080)
	if got, want := s.Start(screen), image.Rect(-1600, 240, -320, 840); got != want {
		t.Errorf("start = %v, want %v", got, want)
	}
	s.Set(image.Rect(10, 20, 110, 220))
	if got := s.Start(screen); got != image.Rect(10, 20, 110, 220) || cfg.SelectionW != 100 {
		t.Errorf("start = %v, saved width %d", got, cfg.SelectionW)
	}
}
