package main

import (
	"image"
	"slices"
	"time"

	"github.com/soocke/pixel-bot-go/domain/detect"
	"github.com/soocke/pixel-bot-go/internal/dataset"
)

const (
	biteEarlyToleranceMS = 200  // labels are only accurate to a frame or two
	biteMatchWindowMS    = 1500 // a later trigger cannot still catch the fish
)

// LocatorCounts and BiteCounts are raw results. They depend only on the frames,
// labels and detectors, so two runs give identical counts.
type LocatorCounts struct {
	Casts         int `json:"casts"`
	Locks         int `json:"locks"`
	LocksOnBobber int `json:"locks_on_bobber"`
	EmptyFrames   int `json:"empty_frames"`
	FalseLocks    int `json:"false_locks"`
}

// BiteCounts holds a bite detector's results.
type BiteCounts struct {
	Casts           int     `json:"casts"`
	BiteCasts       int     `json:"bite_casts"`
	BitesCaught     int     `json:"bites_caught"`
	FalseBites      int     `json:"false_bites"`
	BiteLatenciesMS []int64 `json:"bite_latencies_ms"`
}

// percentile95 uses the nearest-rank method.
func percentile95[T int64 | time.Duration](values []T) T {
	if len(values) == 0 {
		return 0
	}
	sorted := slices.Sorted(slices.Values(values))
	return sorted[(len(sorted)*95+99)/100-1]
}

// evaluateLocator runs a locator over a dataset's casts and bobber-free frames.
func evaluateLocator(l detect.Locator, d *dataset.Dataset, c *LocatorCounts) ([]time.Duration, error) {
	var searches []time.Duration
	search := func(img *image.RGBA) (detect.Match, bool) {
		start := time.Now()
		match, found := l.Locate(img)
		searches = append(searches, time.Since(start))
		return match, found
	}
	for _, cast := range d.Labels.Casts {
		c.Casts++
		for _, f := range d.FramesIn(cast.Span) {
			img, err := f.Load()
			if err != nil {
				return searches, err
			}
			// Stop at the first match, because that is what the app would lock on.
			if match, found := search(img); found {
				c.Locks++
				if image.Pt(match.X, match.Y).In(cast.Bobber.Image()) {
					c.LocksOnBobber++
				}
				break
			}
		}
	}
	for _, span := range d.Labels.Empty {
		for _, f := range d.FramesIn(span) {
			img, err := f.Load()
			if err != nil {
				return searches, err
			}
			c.EmptyFrames++
			if _, found := search(img); found {
				c.FalseLocks++
			}
		}
	}
	return searches, nil
}

// watchAreas finds the area the app would watch in each cast: the marker's box,
// or a square around the labelled bobber when no marker shows. All bite
// detectors see the same areas, so their results compare directly.
func watchAreas(d *dataset.Dataset, roiSize int) ([]image.Rectangle, error) {
	marker := detect.NewMarkerLocator()
	areas := make([]image.Rectangle, len(d.Labels.Casts))
	for i, cast := range d.Labels.Casts {
		b := cast.Bobber.Image()
		half := roiSize / 2
		cx, cy := (b.Min.X+b.Max.X)/2, (b.Min.Y+b.Max.Y)/2
		areas[i] = image.Rect(cx-half, cy-half, cx+half, cy+half)
		for _, f := range d.FramesIn(cast.Span) {
			img, err := f.Load()
			if err != nil {
				return nil, err
			}
			if match, found := marker.Locate(img); found {
				areas[i] = match.Box
				break
			}
		}
	}
	return areas, nil
}

// evaluateBite runs a bite detector over each cast's watch area.
func evaluateBite(b detect.BiteDetector, d *dataset.Dataset, areas []image.Rectangle, c *BiteCounts) error {
	for i, cast := range d.Labels.Casts {
		c.Casts++
		if cast.BiteMS != nil {
			c.BiteCasts++
		}
		b.Reset()
		for _, f := range d.FramesIn(cast.Span) {
			img, err := f.Load()
			if err != nil {
				return err
			}
			if b.Feed(img.SubImage(areas[i]).(*image.RGBA), time.UnixMilli(f.MS)) {
				scoreBite(f.MS, cast.BiteMS, c)
				break
			}
		}
	}
	return nil
}

func scoreBite(triggerMS int64, biteMS *int64, c *BiteCounts) {
	switch {
	case biteMS == nil || triggerMS < *biteMS-biteEarlyToleranceMS:
		c.FalseBites++
	case triggerMS <= *biteMS+biteMatchWindowMS:
		c.BitesCaught++
		c.BiteLatenciesMS = append(c.BiteLatenciesMS, max(triggerMS-*biteMS, 0))
	}
}
