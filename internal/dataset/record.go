package dataset

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/soocke/pixel-bot-go/domain/capture"
)

// recordInterval gives about 10 fps.
// shortcut: live detection runs at 30 fps, so bite results from a recording are
// only an approximation; raise the rate if the disk can take it.
const recordInterval = 100 * time.Millisecond

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
// how many it wrote. File names are milliseconds since start.
func Record(ctx context.Context, frames FrameSource, dir string, start time.Time) (int, error) {
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	ticker := time.NewTicker(recordInterval)
	defer ticker.Stop()
	written := 0
	for {
		select {
		case <-ctx.Done():
			return written, nil
		case <-ticker.C:
		}
		frame, err := frames.Next()
		if err != nil {
			return written, err
		}
		if err := writeFrame(&encoder, dir, frame.CapturedAt.Sub(start).Milliseconds(), frame.Image); err != nil {
			return written, err
		}
		written++
	}
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
