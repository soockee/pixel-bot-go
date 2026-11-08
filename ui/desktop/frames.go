package desktop

import (
	"bytes"
	"image"
	"image/jpeg"
	"net/http"
	"strings"
	"sync"
)

// FramesPath is where Frames serves its images.
const FramesPath = "/frames/"

// Frames keeps the newest JPEG of each preview and serves it at
// FramesPath+name. Put runs on the UI loop, ServeHTTP on webview goroutines.
type Frames struct {
	mu   sync.Mutex
	jpgs map[string][]byte
	buf  bytes.Buffer
}

// NewFrames returns an empty store.
func NewFrames() *Frames { return &Frames{jpgs: map[string][]byte{}} }

// Put encodes img as the newest image of name.
func (f *Frames) Put(name string, img image.Image) {
	f.buf.Reset()
	if err := jpeg.Encode(&f.buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return
	}
	data := bytes.Clone(f.buf.Bytes())
	f.mu.Lock()
	f.jpgs[name] = data
	f.mu.Unlock()
}

// Clear drops every image.
func (f *Frames) Clear() {
	f.mu.Lock()
	clear(f.jpgs)
	f.mu.Unlock()
}

// ServeHTTP serves the newest image of the name in the path.
func (f *Frames) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	data, ok := f.jpgs[strings.TrimPrefix(r.URL.Path, FramesPath)]
	f.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}
