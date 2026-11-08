// Package dataset stores captured frames and their labels for detector evaluation.
//
// A dataset is a directory with frames/<elapsed ms>.png, an optional input.jsonl
// and a labels.json. Label coordinates are pixels in the frame images.
package dataset

import (
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	framesDir     = "frames"
	inputFile     = "input.jsonl"
	labelsFile    = "labels.json"
	partialSuffix = ".tmp"
)

// FramesPath returns the frames directory of a dataset.
func FramesPath(dir string) string { return filepath.Join(dir, framesDir) }

// InputPath returns the input trace of a dataset.
func InputPath(dir string) string { return filepath.Join(dir, inputFile) }

// LatestFrame returns the newest complete frame in a frames directory, or "" if
// there is none. File names sort by time.
func LatestFrame(frames string) string {
	entries, err := os.ReadDir(frames)
	if err != nil {
		return ""
	}
	for _, e := range slices.Backward(entries) {
		if strings.HasSuffix(e.Name(), ".png") {
			return filepath.Join(frames, e.Name())
		}
	}
	return ""
}

// Rect is a labelled area in frame pixels.
type Rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// Image returns the rectangle as an image.Rectangle.
func (r Rect) Image() image.Rectangle { return image.Rect(r.X, r.Y, r.X+r.W, r.Y+r.H) }

// Span is a time range within a recording.
type Span struct {
	StartMS int64 `json:"start_ms"`
	EndMS   int64 `json:"end_ms"`
}

// Cast is a span with the bobber on screen. BiteMS is when the bobber dips, or
// nil if no bite came.
type Cast struct {
	Span
	Bobber Rect   `json:"bobber"`
	BiteMS *int64 `json:"bite_ms,omitempty"`
}

// Labels is the content of labels.json. Empty lists spans with no bobber.
type Labels struct {
	Casts []Cast `json:"casts"`
	Empty []Span `json:"empty"`
}

// FrameRef points at one recorded frame.
type FrameRef struct {
	MS   int64
	Path string
}

// Dataset is a loaded recording. Frames are sorted by time.
type Dataset struct {
	Name   string
	Dir    string
	Labels Labels
	Frames []FrameRef
}

// Load reads and checks the labels and lists the frames of a dataset.
func Load(dir string) (*Dataset, error) {
	data, err := os.ReadFile(filepath.Join(dir, labelsFile))
	if err != nil {
		return nil, err
	}
	d := &Dataset{Name: filepath.Base(dir), Dir: dir}
	if err := json.Unmarshal(data, &d.Labels); err != nil {
		return nil, fmt.Errorf("%s: %w", labelsFile, err)
	}
	if err := d.Labels.check(); err != nil {
		return nil, fmt.Errorf("%s: %w", labelsFile, err)
	}
	entries, err := os.ReadDir(FramesPath(dir))
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), partialSuffix) {
			continue // left by a killed recording
		}
		ms, err := strconv.ParseInt(strings.TrimSuffix(e.Name(), ".png"), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("unexpected file %s in %s", e.Name(), framesDir)
		}
		d.Frames = append(d.Frames, FrameRef{MS: ms, Path: filepath.Join(FramesPath(dir), e.Name())})
	}
	slices.SortFunc(d.Frames, func(a, b FrameRef) int { return int(a.MS - b.MS) })
	return d, nil
}

func (l Labels) check() error {
	for i, c := range l.Casts {
		switch {
		case c.StartMS >= c.EndMS:
			return fmt.Errorf("cast %d: start_ms must be before end_ms", i)
		case c.Bobber.W <= 0 || c.Bobber.H <= 0:
			return fmt.Errorf("cast %d: bobber needs a positive size", i)
		case c.BiteMS != nil && (*c.BiteMS < c.StartMS || *c.BiteMS > c.EndMS):
			return fmt.Errorf("cast %d: bite_ms is outside the cast", i)
		}
	}
	for i, s := range l.Empty {
		if s.StartMS >= s.EndMS {
			return fmt.Errorf("empty span %d: start_ms must be before end_ms", i)
		}
	}
	return nil
}

// FramesIn returns the frames within s, ends included.
func (d *Dataset) FramesIn(s Span) []FrameRef {
	start, _ := slices.BinarySearchFunc(d.Frames, s.StartMS, func(f FrameRef, ms int64) int { return int(f.MS - ms) })
	end, found := slices.BinarySearchFunc(d.Frames, s.EndMS, func(f FrameRef, ms int64) int { return int(f.MS - ms) })
	if found {
		end++
	}
	return d.Frames[start:end]
}

// Load decodes the frame.
func (f FrameRef) Load() (*image.RGBA, error) {
	file, err := os.Open(f.Path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	img, err := png.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", f.Path, err)
	}
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba, nil
	}
	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
	return rgba, nil
}
