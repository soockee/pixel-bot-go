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
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

func TestPickWindow(t *testing.T) {
	const wow = "World of Warcraft"
	tests := []struct {
		name    string
		current string
		chosen  bool
		want    string
	}{
		{"defaults to the game", "", false, wow},
		{"keeps a window the user chose", "Fake Game", true, "Fake Game"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pickWindow(tt.current, tt.chosen); got != tt.want {
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
	bad.CastKey, bad.Locator, bad.RecordDir = "Ctrl", "nope", " "
	if _, err := bad.apply(base); err == nil ||
		!strings.Contains(err.Error(), "Cast key, Locator, Recording folder") {
		t.Errorf("invalid settings: err = %v", err)
	}
}

func TestInteractKeySettings(t *testing.T) {
	base := config.DefaultConfig()
	values := settingsOf(base)
	values.InteractKey = "f"
	saved, err := values.apply(*base)
	if err != nil || saved.ReelMethod != config.ReelRightClick || saved.InteractKey != "f" {
		t.Fatalf("saving Interact before switching methods = %+v, %v", saved, err)
	}
	values = settingsOf(&saved)
	values.ReelMethod = config.ReelInteractKey
	for _, key := range []string{"", "Ctrl", "F3"} {
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

func TestLogoutKeySettings(t *testing.T) {
	base := config.DefaultConfig()
	values := settingsOf(base)
	values.Schedule = true // the Logout key field only shows once breaks are on
	if got, err := values.apply(*base); err != nil || !got.Schedule {
		t.Fatalf("breaks on without a Logout key = %+v, %v", got, err)
	}
	values.LogoutKey = values.CastKey
	if _, err := values.apply(*base); err == nil {
		t.Error("a Logout key equal to the Cast key was accepted")
	}
	values.LogoutKey = "L"
	if got, err := values.apply(*base); err != nil || got.LogoutKey != "L" {
		t.Fatalf("Logout key L = %+v, %v", got, err)
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
	// The board moves the pointer and clicks for the right-click reel.
	values.ESP32Port, values.ReelMethod, values.InteractKey = "auto", config.ReelRightClick, ""
	if got, err := values.apply(*base); err != nil || got.ESP32Port != "AUTO" {
		t.Errorf("right-click reel with the ESP32 = %q, %v; want AUTO", got.ESP32Port, err)
	}
	// The board presses the mouse side buttons too.
	values.CastKey = "Mouse4"
	if _, err := values.apply(*base); err != nil {
		t.Errorf("Mouse 4 for Cast with the ESP32: %v", err)
	}
	values.ReelMethod, values.InteractKey = config.ReelInteractKey, "Mouse5"
	if _, err := values.apply(*base); err != nil {
		t.Errorf("Mouse 5 for Interact with the ESP32: %v", err)
	}
}

func TestSetThemeSavesKnownThemesOnly(t *testing.T) {
	cfg := config.DefaultConfig()
	view := NewView(NewFrames(), func(State) {})
	loop := NewLoop(func() {})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx, time.Hour)
	var icons []string
	path := filepath.Join(t.TempDir(), "config.json")
	s := NewService(loop, view, Commands{SetTheme: func(theme string) { icons = append(icons, theme) }}, nil, cfg,
		path, slog.New(slog.DiscardHandler))

	s.SetTheme("fel")
	s.SetTheme("no-such-theme")
	saved, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme != "fel" || s.State().Theme != "fel" || saved.Theme != "fel" || len(icons) != 1 {
		t.Fatalf("theme %q, shown %q, saved %q, icons %v; want fel everywhere and one icon change",
			cfg.Theme, s.State().Theme, saved.Theme, icons)
	}
}

func TestESP32PortsKeepTheSavedPort(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ESP32Port = "COM7"
	loop := NewLoop(func() {})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx, time.Hour)
	s := NewService(loop, NewView(NewFrames(), func(State) {}), Commands{ListPorts: func() []string { return []string{"COM3"} }},
		nil, cfg, filepath.Join(t.TempDir(), "config.json"), slog.New(slog.DiscardHandler))

	if got := s.ESP32Ports(); !slices.Equal(got, []string{"COM3", "COM7"}) {
		t.Fatalf("ports = %v, want COM3 and the saved COM7", got)
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
	values.CastKey = "nope"
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
