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

// previewWidth is the widest capture the window gets; it shows the capture
// smaller than that, and encoding a whole game window would cost far more.
const previewWidth = 1280

// shrink returns img scaled down by a whole factor to at most width pixels
// wide, sampling every n-th pixel, or img when it is narrow enough.
func shrink(img *image.RGBA, width int) *image.RGBA {
	b := img.Bounds()
	n := (b.Dx() + width - 1) / width
	if n <= 1 {
		return img
	}
	out := image.NewRGBA(image.Rect(0, 0, b.Dx()/n, b.Dy()/n))
	for y := range out.Rect.Dy() {
		row := img.Pix[img.PixOffset(b.Min.X, b.Min.Y+y*n):]
		dst := out.Pix[y*out.Stride:]
		for x := range out.Rect.Dx() {
			copy(dst[x*4:x*4+4], row[x*n*4:x*n*4+4])
		}
	}
	return out
}
