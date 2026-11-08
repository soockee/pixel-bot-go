package cloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// fakeTokens hands out a fixed Google ID token.
type fakeTokens struct{}

func (fakeTokens) Token() (*oauth2.Token, error) {
	return (&oauth2.Token{AccessToken: "a", Expiry: time.Now().Add(time.Hour)}).WithExtra(map[string]any{"id_token": "id-1"}), nil
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUploadSendsRecordingFiles(t *testing.T) {
	retryDelay = time.Millisecond
	dir := filepath.Join(t.TempDir(), "20261108-012132")
	writeFile(t, filepath.Join(dir, "frames", "000000100.png"), "png-1")
	writeFile(t, filepath.Join(dir, "frames", "000000200.png"), "png-22")
	writeFile(t, filepath.Join(dir, "frames", "notes.txt"), "skip")
	writeFile(t, filepath.Join(dir, "audio.wav"), "wav")
	writeFile(t, filepath.Join(dir, "labels.json"), "") // empty: skipped
	writeFile(t, filepath.Join(dir, "other.bin"), "skip")

	var mu sync.Mutex
	stored := map[string]string{"recordings/me/20261108-012132/audio.wav": "wav"} // an earlier attempt
	failedOnce := false
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		object := strings.TrimPrefix(r.URL.Path, "/")
		if r.Header.Get("x-goog-if-generation-match") != "0" || r.ContentLength != mustAtoi(t, strings.Split(r.Header.Get("x-goog-content-length-range"), ",")[0]) {
			http.Error(w, "headers", http.StatusBadRequest)
			return
		}
		if strings.HasSuffix(object, "200.png") && !failedOnce {
			failedOnce = true
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		if _, ok := stored[object]; ok {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		stored[object] = string(body)
	}))
	defer storage.Close()

	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer id-1" || r.URL.Path != "/v1/sessions/20261108-012132/uploads" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		var req struct{ Files []file }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		var resp struct {
			Uploads []signedUpload `json:"uploads"`
		}
		for _, f := range req.Files {
			resp.Uploads = append(resp.Uploads, signedUpload{Path: f.Path, Method: http.MethodPut,
				URL: storage.URL + "/recordings/me/20261108-012132/" + f.Path,
				Headers: map[string]string{
					"Content-Type":                f.ContentType,
					"x-goog-content-length-range": strconv.FormatInt(f.Size, 10) + "," + strconv.FormatInt(f.Size, 10),
					"x-goog-if-generation-match":  "0",
				}})
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer service.Close()

	c := New(service.URL, "client", "secret")
	s := &Session{c: c, tokens: fakeTokens{}}
	var last int
	if err := s.Upload(context.Background(), dir, func(done, total int) { last = done; _ = total }); err != nil {
		t.Fatal(err)
	}
	if last != 3 {
		t.Errorf("progress ended at %d files, want 3", last)
	}
	var objects []string
	for k := range stored {
		objects = append(objects, strings.TrimPrefix(k, "recordings/me/20261108-012132/"))
	}
	slices.Sort(objects)
	if want := []string{"audio.wav", "frames/000000100.png", "frames/000000200.png"}; !slices.Equal(objects, want) {
		t.Errorf("stored %v, want %v", objects, want)
	}
	if stored["recordings/me/20261108-012132/frames/000000200.png"] != "png-22" {
		t.Errorf("retried frame stored as %q", stored["recordings/me/20261108-012132/frames/000000200.png"])
	}
}

func TestUploadReportsRefusal(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "audio.wav"), "wav")
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "your role may not upload recordings", http.StatusForbidden)
	}))
	defer service.Close()
	s := &Session{c: New(service.URL, "client", "secret"), tokens: fakeTokens{}}
	err := s.Upload(context.Background(), dir, func(int, int) {})
	if err == nil || !strings.Contains(err.Error(), "may not upload") {
		t.Errorf("err = %v", err)
	}
}

func TestAccountCan(t *testing.T) {
	a := Account{Email: "x@example.com", Role: "angler", Permissions: []string{PermFish}}
	if !a.Can(PermFish) || a.Can(PermUpload) || (Account{}).Can(PermFish) {
		t.Errorf("Can wrong for %+v", a)
	}
}

func mustAtoi(t *testing.T, s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		t.Errorf("size %q", s)
	}
	return n
}
