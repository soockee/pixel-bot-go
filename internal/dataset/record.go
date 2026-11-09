package dataset

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/soocke/pixel-bot-go/domain/capture"
	"github.com/soocke/pixel-bot-go/domain/detect"
)

// recordInterval gives about 10 fps.
// shortcut: live detection runs at 30 fps, so bite results from a recording are
// only an approximation; raise the rate if the disk can take it.
const recordInterval = 100 * time.Millisecond

const (
	// fullEvery is how often a recording keeps the whole window. In between,
	// frames keep only the square around the bobber, which is all bite
	// detection reads, and leave the rest black, which PNG stores in almost no
	// space; without a bobber they are skipped.
	fullEvery = 10 * time.Second
	// keepSide is the smallest square kept around the bobber: larger than any
	// watch area (roi_size_px), and keepPad more than the marker on each side.
	keepSide = 256
	keepPad  = 32
)

// FrameSource delivers the current image of the watched region.
type FrameSource interface {
	Next() (capture.Frame, error)
}

// Create makes a new dataset directory. It refuses an existing one, so a session
// is never overwritten.
func Create(dir string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		return fmt.Errorf("recording needs a new directory: %w", err)
	}
	return os.Mkdir(FramesPath(dir), 0o755)
}

// Record saves frames into a directory from Create until ctx ends and returns
// how many it wrote. File names are milliseconds since start. Every frame has
// the window's size and coordinates; see fullEvery for what each one holds.
// locator finds the bobber.
func Record(ctx context.Context, frames FrameSource, locator detect.Locator, dir string, start time.Time) (int, error) {
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	ticker := time.NewTicker(recordInterval)
	defer ticker.Stop()
	written := 0
	var lastFull time.Time
	var kept *image.RGBA
	for {
		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
		// A slow frame leaves both ready, and select then picks at random, which
		// would grab one more frame after the stop.
		if ctx.Err() != nil {
			return written, nil
		}
		frame, err := frames.Next()
		if errors.Is(err, capture.ErrFrameUnavailable) {
			continue
		}
		if err != nil {
			return written, err
		}
		img := frame.Image
		if frame.CapturedAt.Sub(lastFull) >= fullEvery {
			lastFull = frame.CapturedAt
		} else if match, found := locator.Locate(img); found {
			kept = keepAround(img, match.Box, kept)
			img = kept
		} else {
			continue
		}
		if err := writeFrame(&encoder, dir, frame.CapturedAt.Sub(start).Milliseconds(), img); err != nil {
			return written, err
		}
		written++
	}
}

// keepAround returns img with only the square around box kept and the rest
// black, reusing buf when it fits. box is relative to img's origin, as
// Locator returns it.
func keepAround(img *image.RGBA, box image.Rectangle, buf *image.RGBA) *image.RGBA {
	if buf == nil || buf.Rect != img.Rect {
		buf = image.NewRGBA(img.Rect)
	} else {
		clear(buf.Pix)
	}
	box = box.Add(img.Rect.Min)
	centre := image.Pt((box.Min.X+box.Max.X)/2, (box.Min.Y+box.Max.Y)/2)
	half := max(keepSide, box.Dx()+2*keepPad, box.Dy()+2*keepPad) / 2
	keep := image.Rect(centre.X-half, centre.Y-half, centre.X+half, centre.Y+half).Intersect(img.Rect)
	draw.Draw(buf, keep, img, keep.Min, draw.Src)
	return buf
}

func writeFrame(encoder *png.Encoder, dir string, ms int64, img *image.RGBA) error {
	// A file gets its final name once complete, so readers never see half a frame.
	path := filepath.Join(FramesPath(dir), fmt.Sprintf("%09d.png", ms))
	file, err := os.Create(path + partialSuffix)
	if err != nil {
		return err
	}
	if err := encoder.Encode(file, img); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(path+partialSuffix, path)
}
