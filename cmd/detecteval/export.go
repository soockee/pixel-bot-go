package main

import (
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"

	"github.com/soocke/pixel-bot-go/internal/dataset"
)

// indexRow describes one exported crop. BiteMS is the labelled estimate.
type indexRow struct {
	File    string       `json:"file"`
	Dataset string       `json:"dataset"`
	Cast    int          `json:"cast"`
	MS      int64        `json:"ms"`
	Box     dataset.Rect `json:"box"`
	ReelMS  int64        `json:"reel_ms"`
	BiteMS  *int64       `json:"bite_ms,omitempty"`
}

// export saves each cast's watch area as small PNGs plus an index.jsonl, which
// is all that training needs and far smaller than the full frames.
func export(out string, dirs []string, roiSize int, status io.Writer) error {
	if len(dirs) == 0 {
		return fmt.Errorf("no datasets given")
	}
	if err := os.Mkdir(out, 0o755); err != nil {
		return fmt.Errorf("export needs a new directory: %w", err)
	}
	indexFile, err := os.Create(filepath.Join(out, "index.jsonl"))
	if err != nil {
		return err
	}
	defer indexFile.Close()
	index := json.NewEncoder(indexFile)
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	crops := 0
	for _, dir := range dirs {
		d, err := dataset.Load(dir)
		if err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		areas, err := watchAreas(d, roiSize)
		if err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		for i, cast := range d.Labels.Casts {
			box := areas[i]
			rel := filepath.Join(d.Name, fmt.Sprintf("cast-%03d", i))
			if err := os.MkdirAll(filepath.Join(out, rel), 0o755); err != nil {
				return err
			}
			// Include the landing, so training can tell it from a bite.
			span := dataset.Span{StartMS: cast.StartMS - dataset.LandingMS, EndMS: cast.EndMS}
			for _, f := range d.FramesIn(span) {
				img, err := f.Load()
				if err != nil {
					return err
				}
				name := filepath.Join(rel, fmt.Sprintf("%09d.png", f.MS))
				file, err := os.Create(filepath.Join(out, name))
				if err != nil {
					return err
				}
				err = encoder.Encode(file, img.SubImage(box))
				if closeErr := file.Close(); err == nil {
					err = closeErr
				}
				if err != nil {
					return err
				}
				row := indexRow{
					File: filepath.ToSlash(name), Dataset: d.Name, Cast: i, MS: f.MS,
					Box:    dataset.Rect{X: box.Min.X, Y: box.Min.Y, W: box.Dx(), H: box.Dy()},
					ReelMS: cast.EndMS, BiteMS: cast.BiteMS,
				}
				if err := index.Encode(row); err != nil {
					return err
				}
				crops++
			}
		}
	}
	fmt.Fprintf(status, "Exported %d crops to %s.\n", crops, out)
	return nil
}
