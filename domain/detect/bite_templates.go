package detect

import (
	"encoding/binary"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/soocke/pixel-bot-go/domain/capture"
)

const (
	templateStride = 8
	templateLength = capture.AudioRate * 80 / 1000 / templateStride
	templateStep   = time.Second * templateStride / capture.AudioRate
)

type biteTemplate struct {
	samples []float64
	offset  time.Duration
	energy  float64
}

type biteTemplates struct {
	templates []biteTemplate
	window    [2 * templateLength]float64
	next      int
	filled    int
	energy    float64
	sum       float64
	n         int
	lastEnd   time.Time
}

func newBiteTemplates(sounds map[string][]byte) *biteTemplates {
	b := &biteTemplates{}
	for _, name := range slices.Sorted(maps.Keys(sounds)) {
		pcm := sounds[name]
		var samples []float64
		for len(pcm) >= templateStride*capture.AudioFrameBytes && len(samples) < capture.AudioRate*180/1000/templateStride {
			var sum float64
			for i := range templateStride {
				sum += monoSample(pcm[i*capture.AudioFrameBytes:])
			}
			samples = append(samples, sum/templateStride)
			pcm = pcm[templateStride*capture.AudioFrameBytes:]
		}
		var best biteTemplate
		for start := 0; start+templateLength <= len(samples); start += templateLength / 8 {
			window := samples[start : start+templateLength]
			var energy float64
			for _, v := range window {
				energy += v * v
			}
			if energy > best.energy {
				best = biteTemplate{window, time.Duration(start) * templateStep, energy}
			}
		}
		if best.energy > 0 {
			b.templates = append(b.templates, best)
		}
	}
	return b
}

func (b *biteTemplates) reset() {
	*b = biteTemplates{templates: b.templates}
}

func monoSample(pcm []byte) float64 {
	l := int16(binary.LittleEndian.Uint16(pcm))
	r := int16(binary.LittleEndian.Uint16(pcm[2:]))
	return (float64(l) + float64(r)) / 2
}

// add matches an 80 ms waveform segment against recent PCM. The returned
// onset accounts for the template's offset within the source asset.
// shortcut: waveform matching assumes stable pitch, try spectral templates if playback changes it.
func (b *biteTemplates) add(pcm []byte, at, splashFrom, splashLast time.Time) (time.Time, bool) {
	frames := len(pcm) / capture.AudioFrameBytes
	if !b.lastEnd.IsZero() && (at.Sub(b.lastEnd) > 5*time.Millisecond || b.lastEnd.Sub(at) > 5*time.Millisecond) {
		b.reset()
	}
	b.lastEnd = at.Add(time.Duration(frames) * time.Second / capture.AudioRate)
	for i := range frames {
		b.sum += monoSample(pcm[i*capture.AudioFrameBytes:])
		b.n++
		if b.n < templateStride {
			continue
		}
		v := b.sum / templateStride
		b.sum, b.n = 0, 0
		old := b.window[b.next]
		b.energy += v*v - old*old
		b.window[b.next], b.window[b.next+templateLength] = v, v
		b.next = (b.next + 1) % templateLength
		b.filled = min(b.filled+1, templateLength)
		if splashFrom.IsZero() || b.filled < templateLength || b.energy < templateLength*soundMinLevel*soundMinLevel {
			continue
		}
		start := at.Add(time.Duration(i+1)*time.Second/capture.AudioRate - templateLength*templateStep)
		window := b.window[b.next : b.next+templateLength]
		for _, template := range b.templates {
			onset := start.Add(-template.offset)
			if onset.Before(splashFrom) || onset.Sub(splashLast) > soundAfterSplash {
				continue
			}
			var dot float64
			for j, sample := range template.samples {
				dot += sample * window[j]
			}
			if dot > 0.72*math.Sqrt(b.energy*template.energy) {
				return onset, true
			}
		}
	}
	return time.Time{}, false
}
