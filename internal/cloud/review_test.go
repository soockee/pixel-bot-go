package cloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/internal/playtrace"
)

func TestReviewedUploadSendsNoKeysAndRequiresMatchingReview(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "frames", "000000100.png"), "frame")
	writeFile(t, filepath.Join(dir, "audio.wav"), "private audio")
	path := filepath.Join(dir, "input.jsonl")
	start := time.Now().UTC()
	trace, err := playtrace.Create(path, playtrace.Header{
		Source: playtrace.Manual, Clock: playtrace.Monotonic, StartedUTC: &start, Keyboard: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, ch := range "pA55w!rd" {
		if err := trace.Write(playtrace.Event{
			Type: "key_down", ElapsedUS: int64(i) * 100_000, Segment: 1,
			Key: &playtrace.Key{Name: string(ch)},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := trace.Write(playtrace.Event{Type: "move", ElapsedUS: 800_000, Segment: 1, Position: &playtrace.Position{X: 50, Y: 80}}); err != nil {
		t.Fatal(err)
	}
	if err := trace.Close(playtrace.End{ElapsedUS: 900_000, KeyEvents: 8, Moves: 1, Status: "complete"}); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	sent := map[string]string{}
	requests := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requests++
		if r.Method == http.MethodPut {
			body, _ := io.ReadAll(r.Body)
			sent[strings.TrimPrefix(r.URL.Path, "/objects/")] = string(body)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/v1/sessions/reviewed-") {
			t.Errorf("upload session = %s", r.URL.Path)
		}
		var req struct{ Files []file }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		var uploads []signedUpload
		for _, file := range req.Files {
			uploads = append(uploads, signedUpload{Path: file.Path, Method: "PUT", URL: server.URL + "/objects/" + file.Path})
		}
		_ = json.NewEncoder(w).Encode(map[string][]signedUpload{"uploads": uploads})
	}))
	defer server.Close()
	session := &Session{c: New(server.URL, "client", "secret"), tokens: fakeTokens{}}
	review, err := ReviewRecording(dir)
	if err != nil {
		t.Fatal(err)
	}
	if review.KeyEvents != 8 || review.SensitiveCount != 1 || len(review.Sensitive) != 1 ||
		review.Sensitive[0].Presses != 8 || review.Frames != 1 || !review.HasAudio {
		t.Fatalf("unexpected review: %+v", review)
	}
	for _, fingerprint := range []string{"", "outdated"} {
		if err := session.UploadReviewed(context.Background(), dir, fingerprint, true, false, func(int, int) {}); err == nil {
			t.Fatal("upload succeeded without matching review")
		}
	}
	mu.Lock()
	beforeUpload := requests
	mu.Unlock()
	if beforeUpload != 0 {
		t.Fatal("review or refusal contacted the upload service")
	}
	if err := session.UploadReviewed(context.Background(), dir, review.Fingerprint, true, false, func(int, int) {}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if _, ok := sent["audio.wav"]; ok || sent["frames/000000100.png"] != "frame" {
		t.Fatalf("unexpected media selection: %v", sent)
	}
	if trace := sent["input.jsonl"]; strings.Contains(trace, "key_down") || strings.Contains(trace, "key_events") ||
		strings.Contains(trace, `"keyboard":true`) || !strings.Contains(trace, `"type":"move"`) {
		t.Fatalf("unexpected uploaded trace: %q", trace)
	}
}
