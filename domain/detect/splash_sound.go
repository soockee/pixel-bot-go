package detect

import (
	"encoding/binary"
	"image"
	"log/slog"
	"math"
	"slices"
	"time"

	"github.com/soocke/pixel-bot-go/domain/capture"
)

// SplashSoundDetector reports a bite when the bobber splashes and the bite sound
// starts shortly after; in recordings the sound follows the splash by 0.2 to
// 0.45 s. Other players' bites sound the same but splash elsewhere, and spell
// effects splash without the sound, so neither alone triggers it.
// Not safe for concurrent use; call Feed and Hear from a single goroutine.
type SplashSoundDetector struct {
	logger *slog.Logger
	scene  brightening
	splash time.Time // last frame with a splash at the bobber
	sound  onsets
	fired  bool
}

// soundAfterSplash is how long after a splash the bite sound may start.
const soundAfterSplash = 800 * time.Millisecond

// NewSplashSoundDetector returns a detector. A nil logger discards logs.
func NewSplashSoundDetector(logger *slog.Logger) *SplashSoundDetector {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &SplashSoundDetector{logger: logger}
}

// Reset forgets the scene and the sound, ready for the next cast.
func (s *SplashSoundDetector) Reset() { *s = SplashSoundDetector{logger: s.logger} }

// Feed takes one frame of the watched box and remembers splashes. Only Hear
// reports bites.
func (s *SplashSoundDetector) Feed(roi *image.RGBA, t time.Time) bool {
	if roi == nil || s.fired {
		return false
	}
	if b, ok := s.scene.measure(roi, t); ok && b.centreSplash() {
		s.splash = t
	}
	return false
}

// Hear takes a packet of capture.CaptureAudio played at t and reports a bite
// once: the start of a sound shortly after a splash.
func (s *SplashSoundDetector) Hear(pcm []byte, t time.Time) bool {
	if s.fired {
		return false
	}
	at, ok := s.sound.add(pcm, t)
	if !ok || s.splash.IsZero() {
		return false
	}
	gap := at.Sub(s.splash)
	s.fired = gap >= 0 && gap <= soundAfterSplash
	if s.fired {
		s.logger.Info("bite detected", "detector", "splash_sound", "sound_after_splash_ms", gap.Milliseconds())
	}
	return s.fired
}

var _ SoundBiteDetector = (*SplashSoundDetector)(nil)

const (
	soundStep       = capture.AudioRate / 40 // 25 ms
	soundHistory    = 80                     // 2 s of steps for the background level
	soundRecent     = 4                      // the last 100 ms are not background yet
	soundMinLevel   = 15                     // RMS of 16-bit samples; quieter is noise
	soundOverGround = 3                      // an onset is this many times the background
)

// onsets finds where sounds start: a 50 ms level, measured every 25 ms, that is
// loud and well above the median of the 2 s before, when the step before was not.
type onsets struct {
	sum      float64 // squares of the current step's samples
	n        int
	prevStep float64 // squares of the step before
	levels   []float64
	loud     bool
}

// add takes a packet whose first sample played at t and reports the time of an onset inside it.
func (o *onsets) add(pcm []byte, t time.Time) (time.Time, bool) {
	var at time.Time
	found := false
	frames := len(pcm) / capture.AudioFrameBytes
	for i := range frames {
		for c := range capture.AudioChannels {
			v := float64(int16(binary.LittleEndian.Uint16(pcm[(i*capture.AudioChannels+c)*2:])))
			o.sum += v * v
		}
		o.n++
		if o.n < soundStep {
			continue
		}
		level := math.Sqrt((o.prevStep + o.sum) / float64(2*soundStep*capture.AudioChannels))
		o.prevStep, o.sum, o.n = o.sum, 0, 0
		loud := o.isLoud(level)
		if loud && !o.loud && !found {
			at = t.Add(time.Duration(i) * time.Second / capture.AudioRate) // t is the packet's first sample
			found = true
		}
		o.loud = loud
		o.levels = append(o.levels, level)
		if len(o.levels) > soundHistory {
			o.levels = o.levels[1:]
		}
	}
	return at, found
}

func (o *onsets) isLoud(level float64) bool {
	if len(o.levels) < soundHistory || level < soundMinLevel {
		return false
	}
	background := slices.Clone(o.levels[:len(o.levels)-soundRecent])
	slices.Sort(background)
	return level >= soundOverGround*background[len(background)/2]
}
