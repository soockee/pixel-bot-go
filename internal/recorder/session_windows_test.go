package recorder

import (
	"bytes"
	"context"
	"image"
	"net/http"
	"net/http/httptest"
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
	return framecapture.Frame{Image: image.NewRGBA(image.Rect(0, 0, 2, 2)), CapturedAt: time.Now()}, nil
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

func TestDashboardServesTheLatestFrame(t *testing.T) {
	const address = "127.0.0.1:6070"
	frames := t.TempDir()
	for name, content := range map[string]string{"000000100.png": "old", "000000200.png": "new", "000000300.png.tmp": "partial"} {
		if err := os.WriteFile(filepath.Join(frames, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	trace := createDashboardTrace(t, "{\"type\":\"move\"}\n")
	get := func(handler http.Handler) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://"+address+"/api/frame", nil))
		return w
	}

	w := get(dashboardHandler(trace, frames, true, address))
	if w.Code != http.StatusOK || w.Body.String() != "new" || w.Header().Get("Content-Type") != "image/png" {
		t.Errorf("status=%d type=%q body=%q", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "img-src 'self'") {
		t.Errorf("CSP blocks the image: %q", w.Header().Get("Content-Security-Policy"))
	}

	// A trace saved without frames has nothing to show.
	if w := get(dashboardHandler(trace, "", false, address)); w.Code != http.StatusNotFound {
		t.Errorf("no frames: status=%d", w.Code)
	}
}
