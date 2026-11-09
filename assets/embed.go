package assets

import (
	"bytes"
	"embed"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"math"
	"path"

	"github.com/jfreymuth/oggvorbis"

	"github.com/soocke/pixel-bot-go/domain/capture"
)

// FishingTargetPNG contains the raw PNG bytes of the fishing target image.
//
//go:embed fishing_target.png
var FishingTargetPNG []byte

// FishingTargetImage decodes the embedded PNG into an image.Image.
func FishingTargetImage() (image.Image, error) {
	if len(FishingTargetPNG) == 0 {
		return nil, fmt.Errorf("embedded fishing_target.png is empty")
	}
	img, err := png.Decode(bytes.NewReader(FishingTargetPNG))
	if err != nil {
		return nil, err
	}
	return img, nil
}

// BiteSounds holds the game's fishing bite sounds, Ogg Vorbis, one per file
// in sounds/. The fake game plays them; detection tests feed them through the
// bite detector.
//
//go:embed sounds/*.ogg
var BiteSounds embed.FS

// BiteSoundPCM decodes every bite sound into the layout capture.CaptureAudio
// delivers: capture.AudioRate, 16-bit little-endian, stereo.
func BiteSoundPCM() (map[string][]byte, error) {
	files, err := fs.Glob(BiteSounds, "sounds/*.ogg")
	if err != nil {
		return nil, err
	}
	sounds := map[string][]byte{}
	for _, name := range files {
		data, err := BiteSounds.ReadFile(name)
		if err != nil {
			return nil, err
		}
		samples, format, err := oggvorbis.ReadAll(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		sounds[path.Base(name)] = toCapturePCM(samples, format.Channels, format.SampleRate)
	}
	return sounds, nil
}

// toCapturePCM resamples interleaved float samples linearly to capture.AudioRate and
// writes each frame's channel mean to both stereo channels as int16.
func toCapturePCM(samples []float32, channels, rate int) []byte {
	const outRate = capture.AudioRate
	frames := len(samples) / channels
	mono := make([]float32, frames)
	for i := range frames {
		for c := range channels {
			mono[i] += samples[i*channels+c] / float32(channels)
		}
	}
	n := frames * outRate / rate
	pcm := make([]byte, 0, n*4)
	for i := range n {
		pos := float64(i) * float64(rate) / outRate
		j := int(pos)
		v := mono[j]
		if j+1 < frames {
			v += (mono[j+1] - mono[j]) * float32(pos-float64(j))
		}
		s := int16(max(-1, min(1, v)) * math.MaxInt16)
		pcm = binary.LittleEndian.AppendUint16(pcm, uint16(s))
		pcm = binary.LittleEndian.AppendUint16(pcm, uint16(s))
	}
	return pcm
}
