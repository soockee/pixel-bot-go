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
//
// A splash is a bright splash at the centre, or a clear change there for the
// water's own level (centreMotion), which also catches the small splash of a
// far bobber.
//
// Two fallbacks cover games that never play the bite sound. A game that has
// made no sound at all since the run started, for example with its sound
// switched off, takes the change at the centre alone as the bite. A game that
// makes sound but no bite sound, for example with sound effects off and music
// on, is recognised after unansweredCasts casts in a row whose splash brought
// no bite; from then on the splash alone reports the bite. Neither applies
// once the run has heard the game, or a bite sound: a game with sound effects
// on is silent between them, and in recorded hand play every bite had its
// sound. The engine builds a detector per run, so the run's memory starts
// again at Start.
type SplashSoundDetector struct {
	logger     *slog.Logger
	scene      brightening
	motion     centreMotion
	start      time.Time // the first frame since Reset
	splashFrom time.Time // first frame of the latest splash at the bobber
	splash     time.Time // its last frame
	sound      onsets
	fired      bool

	// Kept across casts for the run.
	heardGame   bool // the game made some sound
	heardBite   bool // a bite was confirmed by its sound
	unanswered  int  // casts in a row whose splash brought no bite
	noBiteSound bool // the game plays no bite sound
}

const (
	splashSettle    = 3 * time.Second        // landing ripples fade within this time after the lock
	splashRise      = 35                     // luminance above the baseline that counts as brightened
	splashLag       = 500 * time.Millisecond // the baseline takes on lasting changes, such as the landed bobber or a parked cursor
	splashMinCentre = 0.04                   // share of the centre disc that must brighten
)

const (
	// soundAfterSplash is how long after a splash the bite sound may start.
	soundAfterSplash = 800 * time.Millisecond
	// SilentFor is how long the game must be silent before a splash alone
	// counts; tests wait this long after a lock before a silent bite.
	SilentFor = 3 * time.Second
	// unansweredCasts is how many casts in a row must see a splash without a
	// bite before a game that makes sound counts as one without the bite
	// sound. One is too few: a cast that runs out, or another player's spell,
	// also leaves an unanswered splash.
	unansweredCasts = 3
)

// NewSplashSoundDetector returns a detector. A nil logger discards logs.
func NewSplashSoundDetector(logger *slog.Logger) *SplashSoundDetector {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &SplashSoundDetector{logger: logger}
}

// Reset forgets the scene and the sound, ready for the next cast. It keeps
// what the run learned about the game's sound.
func (s *SplashSoundDetector) Reset() {
	run := SplashSoundDetector{
		logger:      s.logger,
		heardGame:   s.heardGame || !s.sound.audible.IsZero(),
		heardBite:   s.heardBite,
		unanswered:  s.unanswered,
		noBiteSound: s.noBiteSound,
	}
	switch {
	case s.fired:
		run.unanswered = 0
	case !s.splash.IsZero():
		run.unanswered++
	}
	if !run.noBiteSound && !run.heardBite && run.heardGame && run.unanswered >= unansweredCasts {
		run.noBiteSound = true
		s.logger.Info("no bite sound after splashes in several casts; the splash alone reports bites for the rest of the run",
			"casts", run.unanswered)
	}
	*s = run
}

// Feed takes one frame of the watched box and remembers splashes. Only Hear
// reports bites.
func (s *SplashSoundDetector) Feed(roi *image.RGBA, t time.Time) bool {
	if roi == nil || s.fired {
		return false
	}
	if s.start.IsZero() {
		s.start = t
	}
	b, settled := s.scene.measure(roi, t)
	m, moved := s.motion.observe(roi, t)
	moved = moved && t.Sub(s.start) >= splashSettle // the same settle as the bright splash
	bright := settled && b.centreSplash()
	if !bright && !moved {
		return false
	}
	if s.splash.IsZero() || t.Sub(s.splash) > soundAfterSplash {
		s.splashFrom = t // a new splash
	}
	s.splash = t
	// Spell effects of other players brighten the whole box, so a bright splash
	// alone needs at least half of the brightened pixels in the centre.
	if s.noBiteSound && (moved || 2*b.centre >= b.all) {
		s.fired = true
		s.logger.Info("bite detected", "detector", "splash_sound", "without_bite_sound", true,
			"change", m.change, "centre", b.centre, "brightened", b.all)
		return true
	}
	// A silent game: nothing heard this run, and at least SilentFor listened.
	if !moved || t.Sub(s.start) < SilentFor || s.heardGame || !s.sound.audible.IsZero() {
		return false
	}
	s.fired = true
	s.logger.Info("bite detected", "detector", "splash_sound", "without_sound", true, "change", m.change, "threshold", m.threshold)
	return true
}

// Hear takes a packet of capture.CaptureAudio played at t and reports a bite
// once: the start of a sound shortly after a splash. A splash lasts several
// frames and the sound often starts while it still shows, so the sound counts
// from the splash's first frame until soundAfterSplash after its last. Sound
// arrives a little after the frames of the same moment, so measuring from the
// last frame alone would put the sound before the splash.
func (s *SplashSoundDetector) Hear(pcm []byte, t time.Time) bool {
	if s.fired {
		return false
	}
	at, ok := s.sound.add(pcm, t)
	if !ok || s.splash.IsZero() {
		return false
	}
	gap := at.Sub(s.splashFrom)
	s.fired = gap >= 0 && at.Sub(s.splash) <= soundAfterSplash
	if s.fired {
		s.heardBite = true
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
	soundAudible    = 1                      // any game sound; recorded fishing has a median of 6, a game with its sound off 0
	soundOverGround = 3                      // an onset is this many times the background
	// The bite sound swells over 200 to 500 ms, so it crosses soundOverGround
	// up to 300 ms after it starts when the game is noisy. An onset is dated
	// back over the steps before it that already rose soundRise times over the
	// background, at most soundLookback of them.
	soundRise     = 1.5
	soundLookback = 20 // 500 ms
)

// onsets finds where sounds start: a 50 ms level, measured every 25 ms, that is
// loud and well above the median of the 2 s before, when the step before was
// not. The onset's time is where that rise began.
type onsets struct {
	sum      float64 // squares of the current step's samples
	n        int
	prevStep float64 // squares of the step before
	levels   []float64
	times    []time.Time // when each of levels ended
	loud     bool
	audible  time.Time // when the game last made any sound
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
		now := t.Add(time.Duration(i) * time.Second / capture.AudioRate) // t is the packet's first sample
		if level >= soundAudible {
			o.audible = now
		}
		background, ok := o.background()
		// A sound stays one sound until it falls back near the background, so
		// the swells inside one bite sound do not start new ones.
		loud := ok && (level >= soundMinLevel && level >= soundOverGround*background || o.loud && level > soundRise*background)
		if loud && !o.loud && !found {
			at, found = o.riseStart(now, background), true
		}
		o.loud = loud
		o.levels = append(o.levels, level)
		o.times = append(o.times, now)
		if len(o.levels) > soundHistory {
			o.levels, o.times = o.levels[1:], o.times[1:]
		}
	}
	return at, found
}

// background is the median level of the 2 s before the last 100 ms, false
// until 2 s have been heard.
func (o *onsets) background() (float64, bool) {
	if len(o.levels) < soundHistory {
		return 0, false
	}
	b := slices.Clone(o.levels[:len(o.levels)-soundRecent])
	slices.Sort(b)
	return b[len(b)/2], true
}

// riseStart returns the end of the earliest of the steps just before now that
// already rose over the background, or now when none did.
func (o *onsets) riseStart(now time.Time, background float64) time.Time {
	start := now
	for j := len(o.levels) - 1; j >= max(0, len(o.levels)-soundLookback); j-- {
		if o.levels[j] <= soundRise*background {
			break
		}
		start = o.times[j]
	}
	return start
}

// brightening compares each frame of the watched box with a baseline that
// follows the scene with a lag, so an abrupt splash stands out.
type brightening struct {
	base        []float32
	w, h        int
	start, last time.Time
}

// brightened counts pixels that rose by splashRise, in the whole box and in the
// centre disc where the bobber sits.
type brightened struct{ all, centre, disc int }

func (b brightened) centreSplash() bool {
	return b.centre > 0 && float64(b.centre) >= splashMinCentre*float64(b.disc)
}

// measure updates the baseline with roi and reports what brightened. It reports
// false until the scene has settled after the first frame.
func (s *brightening) measure(roi *image.RGBA, t time.Time) (brightened, bool) {
	r := roi.Bounds()
	w, h := r.Dx(), r.Dy()
	first := s.base == nil || w != s.w || h != s.h
	if first {
		s.base, s.w, s.h, s.start = make([]float32, w*h), w, h, t
	}
	follow := float32(1 - math.Exp(-float64(t.Sub(s.last))/float64(splashLag)))
	s.last = t
	cx, cy, radius := w/2, h/2, min(w, h)/4
	var b brightened
	for y := range h {
		row := roi.Pix[y*roi.Stride:]
		for x := range w {
			p := row[x*4:]
			l := float32(77*uint32(p[0])+150*uint32(p[1])+29*uint32(p[2])) / 256
			i := y*w + x
			if first {
				s.base[i] = l
				continue
			}
			inDisc := (x-cx)*(x-cx)+(y-cy)*(y-cy) <= radius*radius
			if inDisc {
				b.disc++
			}
			if l-s.base[i] > splashRise {
				b.all++
				if inDisc {
					b.centre++
				}
			}
			s.base[i] += (l - s.base[i]) * follow
		}
	}
	return b, !first && t.Sub(s.start) >= splashSettle
}
