package desktop

import (
	"context"
	"image"
	"image/color"
	"log/slog"
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

func TestInteractKeySettings(t *testing.T) {
	base := config.DefaultConfig()
	values := settingsOf(base)
	values.ReelMethod = config.ReelInteractKey
	for _, key := range []string{"", "Ctrl", "F3", "F10"} {
		values.InteractKey = key
		if _, err := values.apply(*base); err == nil {
			t.Errorf("Interact key %q accepted", key)
		}
	}
	values.InteractKey = "f"
	got, err := values.apply(*base)
	if err != nil || got.ReelMethod != config.ReelInteractKey || got.InteractKey != "f" {
		t.Fatalf("Interact settings = %+v, %v", got, err)
	}
}

func TestESP32PortSettings(t *testing.T) {
	base := config.DefaultConfig()
	values := settingsOf(base)
	values.ReelMethod, values.InteractKey = config.ReelInteractKey, "f"
	for in, want := range map[string]string{"auto": "AUTO", " Auto ": "AUTO", "com3": "COM3", "": ""} {
		values.ESP32Port = in
		got, err := values.apply(*base)
		if err != nil || got.ESP32Port != want {
			t.Errorf("ESP32 port %q = %q, %v; want %q", in, got.ESP32Port, err, want)
		}
	}
	values.ESP32Port = "ttyUSB0"
	if _, err := values.apply(*base); err == nil || !strings.Contains(err.Error(), "auto") {
		t.Errorf("a name that is not a COM port or auto was accepted: %v", err)
	}
	values.ESP32Port, values.ReelMethod = "auto", config.ReelRightClick
	if _, err := values.apply(*base); err == nil {
		t.Error("auto port accepted without the Interact key reel method")
	}
}

func TestShareRecordingsShowsTheRecorder(t *testing.T) {
	cfg := config.DefaultConfig()
	view := NewView(NewFrames(), func(State) {})
	loop := NewLoop(func() {})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx, time.Hour)
	s := NewService(loop, view, Commands{Running: func() bool { return false }}, nil, cfg,
		filepath.Join(t.TempDir(), "config.json"), slog.New(slog.DiscardHandler))

	values := settingsOf(cfg)
	values.ShareRecordings = true
	if _, err := s.SaveSettings(values); err != nil {
		t.Fatal(err)
	}
	if !cfg.ShareRecordings || !s.State().Recorder {
		t.Fatalf("saved %v, recorder shown %v; want both", cfg.ShareRecordings, s.State().Recorder)
	}
	values.ShareRecordings = false
	if _, err := s.SaveSettings(values); err != nil || s.State().Recorder {
		t.Fatalf("switched off: err %v, recorder shown %v", err, s.State().Recorder)
	}
}

func TestSavingSettingsRechecksTheAddon(t *testing.T) {
	cfg := config.DefaultConfig()
	loop := NewLoop(func() {})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx, time.Hour)
	checks := 0
	cmds := Commands{Running: func() bool { return false }, CheckAddon: func() { checks++ }}
	s := NewService(loop, NewView(NewFrames(), func(State) {}), cmds, nil, cfg,
		filepath.Join(t.TempDir(), "config.json"), slog.New(slog.DiscardHandler))

	values := settingsOf(cfg)
	values.CastKey = "F4" // the addon binds this key, so its install is now out of date
	if _, err := s.SaveSettings(values); err != nil {
		t.Fatal(err)
	}
	if checks != 1 {
		t.Fatalf("addon checked %d times after saving, want 1", checks)
	}
	values.StopKey = "nope"
	if _, err := s.SaveSettings(values); err == nil || checks != 1 {
		t.Fatalf("a rejected save: err %v, checks %d; want an error and no check", err, checks)
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

func TestShrinkKeepsPreviewsSmall(t *testing.T) {
	game := image.NewRGBA(image.Rect(0, 0, 3440, 1440))
	game.Set(3439, 1439, color.RGBA{R: 255, A: 255})
	if got := shrink(game, previewWidth).Bounds().Size(); got != image.Pt(1146, 480) {
		t.Errorf("3440x1440 shrinks to %v, want 1146x480", got)
	}
	small := image.NewRGBA(image.Rect(0, 0, 560, 420))
	if shrink(small, previewWidth) != small {
		t.Error("a narrow capture was copied")
	}
}

func TestUploadRefusesUnsafeNames(t *testing.T) {
	var uploaded []string
	s := &Service{recordings: NewRecordings(func() string { return "datasets" }),
		cmds: Commands{Upload: func(dir string) error { uploaded = append(uploaded, dir); return nil }}}
	for _, name := range []string{"", ".", "..", `..\x`, "a/b"} {
		if err := s.Upload(name); err == nil {
			t.Errorf("Upload(%q) accepted", name)
		}
	}
	if err := s.Upload("20261108-012132"); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("datasets", "20261108-012132"); len(uploaded) != 1 || uploaded[0] != want {
		t.Errorf("uploaded %v, want [%s]", uploaded, want)
	}
}
