package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

const (
	// batchSize stays well under the service's 1000 per request, so each
	// batch's URLs (valid 15 minutes) are used soon after they are signed.
	batchSize = 200
	workers   = 4
)

// retryDelay is the first wait before retrying a transient failure; it doubles.
var retryDelay = time.Second

// contentTypes are the recording files the service accepts (internal/dataset
// layout); frames are matched by framePattern. Keep in step with the
// service's rules in vrl-recordings/upload/server.go.
var contentTypes = map[string]string{
	"input.jsonl":  "application/x-ndjson",
	"audio.wav":    "audio/wav",
	"session.json": "application/json",
	"labels.json":  "application/json",
}

var framePattern = regexp.MustCompile(`^[0-9]{1,12}\.png$`)

type file struct {
	Path        string `json:"path"` // slash-separated, relative to the recording
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	local       string
}

type signedUpload struct {
	Path    string            `json:"path"`
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

// recordingFiles lists the files of the recording in dir the service takes.
// Anything else, and empty files, are left out.
func recordingFiles(dir string) ([]file, error) {
	var files []file
	add := func(rel, contentType string) error {
		local := filepath.Join(dir, filepath.FromSlash(rel))
		info, err := os.Lstat(local)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.Mode().IsRegular() && info.Size() > 0 {
			files = append(files, file{Path: rel, Size: info.Size(), ContentType: contentType, local: local})
		}
		return nil
	}
	for name, ct := range contentTypes {
		if err := add(name, ct); err != nil {
			return nil, err
		}
	}
	framesDir := filepath.Join(dir, "frames")
	if info, err := os.Lstat(framesDir); err == nil && !info.IsDir() {
		return nil, fmt.Errorf("recording frames must be a local directory")
	}
	frames, err := os.ReadDir(framesDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, e := range frames {
		if framePattern.MatchString(e.Name()) {
			if err := add("frames/"+e.Name(), "image/png"); err != nil {
				return nil, err
			}
		}
	}
	return files, nil
}

// uploadFiles sends a previously reviewed manifest.
func (s *Session) uploadFiles(ctx context.Context, session string, files []file, progress func(done, total int)) error {
	// Workers finish files concurrently; the lock keeps progress calls one at
	// a time and in order.
	var mu sync.Mutex
	done := 0
	fileDone := func() {
		mu.Lock()
		defer mu.Unlock()
		done++
		progress(done, len(files))
	}
	for start := 0; start < len(files); start += batchSize {
		batch := files[start:min(start+batchSize, len(files))]
		uploads, err := s.sign(ctx, session, batch)
		if err != nil {
			return err
		}
		if err := s.putAll(ctx, batch, uploads, fileDone); err != nil {
			return err
		}
	}
	return nil
}

// sign asks the service for one signed URL per file.
func (s *Session) sign(ctx context.Context, session string, batch []file) ([]signedUpload, error) {
	body, err := json.Marshal(map[string][]file{"files": batch})
	if err != nil {
		return nil, err
	}
	req, err := s.request(ctx, http.MethodPost, "/v1/sessions/"+session+"/uploads", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	var resp struct {
		Uploads []signedUpload `json:"uploads"`
	}
	if err := s.c.do(req, &resp); err != nil {
		return nil, err
	}
	if len(resp.Uploads) != len(batch) {
		return nil, fmt.Errorf("upload service signed %d of %d files", len(resp.Uploads), len(batch))
	}
	return resp.Uploads, nil
}

// putAll uploads a batch with a few workers and stops at the first failure.
func (s *Session) putAll(ctx context.Context, batch []file, uploads []signedUpload, fileDone func()) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for i := range jobs {
				if err := put(ctx, s.c.http, batch[i], uploads[i]); err != nil {
					cancel(err)
					continue
				}
				fileDone()
			}
		})
	}
	for i := range batch {
		select {
		case jobs <- i:
		case <-ctx.Done():
		}
	}
	close(jobs)
	wg.Wait()
	if err := context.Cause(ctx); err != nil && err != context.Canceled {
		return err
	}
	return ctx.Err()
}

// put uploads one file to its signed URL, retrying transient failures. 412
// means an earlier attempt already stored it.
func put(ctx context.Context, client *http.Client, f file, u signedUpload) error {
	var last error
	for attempt := range 4 {
		if attempt > 0 {
			select {
			case <-time.After(retryDelay << (attempt - 1)):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		status, err := putOnce(ctx, client, f, u)
		switch {
		case err == nil && (status == http.StatusOK || status == http.StatusPreconditionFailed):
			return nil
		case err == nil && status != http.StatusTooManyRequests && status < 500:
			return fmt.Errorf("upload %s: storage answered %d", f.Path, status)
		case err != nil && ctx.Err() != nil:
			return ctx.Err()
		}
		last = fmt.Errorf("upload %s: status %d, %v", f.Path, status, err)
	}
	return last
}

func putOnce(ctx context.Context, client *http.Client, f file, u signedUpload) (int, error) {
	body, err := os.Open(f.local)
	if err != nil {
		return 0, err
	}
	defer body.Close()
	req, err := http.NewRequestWithContext(ctx, u.Method, u.URL, body)
	if err != nil {
		return 0, err
	}
	req.ContentLength = f.Size // the signature binds the exact size
	for k, v := range u.Headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode, nil
}
