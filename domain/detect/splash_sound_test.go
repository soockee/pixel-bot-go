package detect

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/assets"
	"github.com/soocke/pixel-bot-go/domain/capture"
)

// tone is a 10 ms packet whose samples all have the given amplitude.
func tone(amplitude int16) []byte {
	pcm := make([]byte, capture.AudioRate/100*capture.AudioFrameBytes)
	for i := 0; i < len(pcm); i += 2 {
		binary.LittleEndian.PutUint16(pcm[i:], uint16(amplitude))
	}
	return pcm
}

// listen plays 7 s of calm water and quiet sound, with a splash frame and a
// 300 ms sound at the given times (negative for none), and returns when the
// detector fired.
func listen(d *SplashSoundDetector, splashAt, soundAt time.Duration) (time.Duration, bool) {
	start := time.Unix(0, 0)
	quiet, loud := tone(5), tone(200)
	for t := time.Duration(0); t < 7*time.Second; t += 10 * time.Millisecond {
		if t%(100*time.Millisecond) == 0 {
			frame := water(0, 0, 0)
			if t == splashAt {
				frame = water(50, 45, 10)
			}
			if d.Feed(frame, start.Add(t)) {
				return t, true
			}
		}
		packet := quiet
		if soundAt >= 0 && t >= soundAt && t < soundAt+300*time.Millisecond {
			packet = loud
		}
		if d.Hear(packet, start.Add(t)) {
			return t, true
		}
	}
	return 0, false
}

func TestSplashSound(t *testing.T) {
	const s = time.Second
	tests := []struct {
		name            string
		splashAt, sound time.Duration
		want            bool
	}{
		{"splash, then the bite sound", 4 * s, 4*s + 300*time.Millisecond, true},
		{"another player's bite: sound only", -1, 4 * s, false},
		{"spell effect: splash only", 4 * s, -1, false},
		{"sound too long after the splash", 4 * s, 5 * s, false},
		{"sound before the splash", 4 * s, 3*s + 800*time.Millisecond, false},
		{"landing splash before the scene settles", 2 * s, 2*s + 300*time.Millisecond, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at, fired := listen(NewSplashSoundDetector(nil), tt.splashAt, tt.sound)
			if fired != tt.want {
				t.Fatalf("fired = %v at %v, want %v", fired, at, tt.want)
			}
			if fired && (at < tt.sound || at > tt.sound+50*time.Millisecond) { // levels come in 25 ms steps
				t.Errorf("fired at %v, want within 50 ms of the sound at %v", at, tt.sound)
			}
		})
	}
}

func TestSplashSoundFiresOnceUntilReset(t *testing.T) {
	d := NewSplashSoundDetector(nil)
	if _, fired := listen(d, 4*time.Second, 4300*time.Millisecond); !fired {
		t.Fatal("no bite")
	}
	if d.Hear(tone(200), time.Unix(8, 0)) {
		t.Error("fired twice")
	}
	d.Reset()
	if _, fired := listen(d, 4*time.Second, 4300*time.Millisecond); !fired {
		t.Error("no bite after Reset")
	}
}

// water returns a calm 100x90 frame with an optional bright disc at (x, y).
func water(x, y, radius int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 100, 90))
	for py := range 90 {
		for px := range 100 {
			c := color.RGBA{50, 60, 70, 255}
			if (px-x)*(px-x)+(py-y)*(py-y) <= radius*radius {
				c = color.RGBA{180, 190, 200, 255}
			}
			img.SetRGBA(px, py, c)
		}
	}
	return img
}

// With sound effects off and music on, the game makes sound but no bite
// sound: the first cast's splash goes unanswered, and from the next cast on
// the splash alone reports the bite.
func TestSplashSoundLearnsAGameWithoutBiteSound(t *testing.T) {
	d := NewSplashSoundDetector(nil)
	for cast := range unansweredCasts {
		if _, fired := listen(d, 4*time.Second, -1); fired {
			t.Fatalf("fired on splash %d without a bite sound", cast+1)
		}
		d.Reset()
	}
	at, fired := listen(d, 4*time.Second, -1)
	if !fired || at != 4*time.Second {
		t.Fatalf("fired = %v at %v, want the splash at 4s", fired, at)
	}
	d.Reset()
	if _, fired := listen(d, -1, -1); fired {
		t.Error("fired on calm water")
	}
}

// Once a bite was confirmed by its sound, the game plays the bite sound:
// unanswered splashes later in the run, from casts that ran out or other
// players' spells, never switch to the splash alone.
func TestSplashSoundKeepsNeedingTheSoundAfterAHeardBite(t *testing.T) {
	d := NewSplashSoundDetector(nil)
	if _, fired := listen(d, 4*time.Second, 4300*time.Millisecond); !fired {
		t.Fatal("missed the bite with its sound")
	}
	for range 2 * unansweredCasts {
		d.Reset()
		if at, fired := listen(d, 4*time.Second, -1); fired {
			t.Fatalf("fired at %v on a splash without a sound", at)
		}
	}
}

// A game with sound effects on is silent between them. Once the run heard the
// game, a quiet stretch in a later cast is no reason to take motion alone.
func TestSplashSoundIgnoresSilenceOnceTheGameWasHeard(t *testing.T) {
	d := NewSplashSoundDetector(nil)
	start := time.Unix(0, 0)
	for at := time.Duration(0); at < 500*time.Millisecond; at += 10 * time.Millisecond {
		d.Hear(tone(200), start.Add(at)) // the cast sound
	}
	d.Reset()
	if at, fired := watch(d, 7*time.Second, func(at time.Duration) *image.RGBA {
		if at >= 4*time.Second {
			return pond(rand.New(rand.NewPCG(1, 2)), 100, 90, 30, 0)
		}
		return pond(rand.New(rand.NewPCG(3, 4)), 100, 90, 0, 0)
	}); fired {
		t.Errorf("fired at %v on motion alone after the game was heard", at)
	}
}

// A cast without a splash, such as one the bot stopped early, teaches nothing.
func TestSplashSoundKeepsNeedingTheSoundAfterACalmCast(t *testing.T) {
	d := NewSplashSoundDetector(nil)
	listen(d, -1, -1)
	d.Reset()
	if _, fired := listen(d, 4*time.Second, -1); fired {
		t.Fatal("fired on a splash alone after a calm cast")
	}
}

// game plays the bite sound clip at gain from soundAt on, over a hum of
// game sound at level hum, as 10 ms packets from time zero.
type game struct {
	clip    []byte
	gain    float64
	hum     float64
	soundAt time.Duration
	rng     *rand.Rand
}

func newGame(t *testing.T, name string, gain, hum float64, soundAt time.Duration) *game {
	t.Helper()
	sounds, err := assets.BiteSoundPCM()
	if err != nil {
		t.Fatal(err)
	}
	clip, ok := sounds[name]
	if !ok {
		t.Fatalf("no bite sound %s", name)
	}
	return &game{clip: clip, gain: gain, hum: hum, soundAt: soundAt, rng: rand.New(rand.NewPCG(1, 2))}
}

// packet returns the 10 ms of sound that start at.
func (g *game) packet(at time.Duration) []byte {
	const frames = capture.AudioRate / 100
	first := int(at * capture.AudioRate / time.Second)
	clipStart := int(g.soundAt * capture.AudioRate / time.Second)
	pcm := make([]byte, frames*capture.AudioFrameBytes)
	for k := range frames {
		v := g.rng.NormFloat64() * g.hum
		if j := first + k - clipStart; j >= 0 && j < len(g.clip)/capture.AudioFrameBytes {
			v += float64(int16(binary.LittleEndian.Uint16(g.clip[j*capture.AudioFrameBytes:]))) * g.gain
		}
		s := uint16(int16(max(math.MinInt16, min(math.MaxInt16, v))))
		binary.LittleEndian.PutUint16(pcm[k*4:], s)
		binary.LittleEndian.PutUint16(pcm[k*4+2:], s)
	}
	return pcm
}

var biteSounds = []string{"FishingBobber_ver2_1.ogg", "FishingBobber_ver2_2.ogg", "FishingBobber_ver2_3.ogg"}

// Each recorded bite sound is one onset, dated near where it starts, loud or
// faint: its swells later on start no new sound.
func TestBiteSoundsAreOneOnset(t *testing.T) {
	levels := []struct{ gain, hum float64 }{{1, 0}, {1, 6}, {1, 30}, {0.1, 6}, {0.1, 30}, {0.01, 0}, {0.01, 6}}
	for _, name := range biteSounds {
		for _, l := range levels {
			t.Run(fmt.Sprintf("%s gain %v hum %v", name, l.gain, l.hum), func(t *testing.T) {
				g := newGame(t, name, l.gain, l.hum, 3*time.Second)
				var o onsets
				var heard []time.Duration
				for at := time.Duration(0); at < 8*time.Second; at += 10 * time.Millisecond {
					if onset, ok := o.add(g.packet(at), time.Unix(0, 0).Add(at)); ok {
						heard = append(heard, onset.Sub(time.Unix(0, 0))-g.soundAt)
					}
				}
				if len(heard) != 1 || heard[0] < 0 || heard[0] > 175*time.Millisecond {
					t.Errorf("onsets %v after the sound started, want one within 175 ms", heard)
				}
			})
		}
	}
}

// listenGame plays 7 s of calm water with a splash frame at splashAt and the
// game's sound, and returns when the detector fired.
func listenGame(d *SplashSoundDetector, g *game, splashAt time.Duration) (time.Duration, bool) {
	start := time.Unix(0, 0)
	for t := time.Duration(0); t < 7*time.Second; t += 10 * time.Millisecond {
		if t%(100*time.Millisecond) == 0 {
			frame := water(0, 0, 0)
			if t == splashAt {
				frame = water(50, 45, 10)
			}
			if d.Feed(frame, start.Add(t)) {
				return t, true
			}
		}
		if d.Hear(g.packet(t), start.Add(t)) {
			return t, true
		}
	}
	return 0, false
}

func TestSplashSoundHearsRecordedBiteSounds(t *testing.T) {
	const splash = 4 * time.Second
	for _, name := range biteSounds {
		for _, delay := range []time.Duration{200 * time.Millisecond, 450 * time.Millisecond} {
			t.Run(fmt.Sprintf("%s %v after the splash", name, delay), func(t *testing.T) {
				at, fired := listenGame(NewSplashSoundDetector(nil), newGame(t, name, 0.1, 6, splash+delay), splash)
				if !fired || at < splash+delay || at > splash+delay+200*time.Millisecond {
					t.Errorf("fired = %v at %v, want within 200 ms of the sound at %v", fired, at, splash+delay)
				}
			})
		}
		// Another player's bite sound began just before the splash; its swells
		// must not pass for the player's own bite sound.
		t.Run(name+" started before the splash", func(t *testing.T) {
			if at, fired := listenGame(NewSplashSoundDetector(nil), newGame(t, name, 0.1, 6, splash-300*time.Millisecond), splash); fired {
				t.Errorf("fired at %v on a sound that began before the splash", at)
			}
		})
	}
}

// The splash shows for 600 ms, frames come at the app's 30 fps, and the sound
// arrives 50 ms after the frames of the same moment, as the fake game showed.
// The bite sound starts while the splash still shows.
func TestSplashSoundHearsASoundThatStartsDuringTheSplash(t *testing.T) {
	const splash, lag = 4 * time.Second, 50 * time.Millisecond
	for _, name := range biteSounds {
		t.Run(name, func(t *testing.T) {
			d := NewSplashSoundDetector(nil)
			g := newGame(t, name, 1, 9, splash+300*time.Millisecond)
			start := time.Unix(0, 0)
			nextFrame := time.Duration(0)
			for at := time.Duration(0); at < 7*time.Second; at += 10 * time.Millisecond {
				for ; nextFrame <= at; nextFrame += 33 * time.Millisecond {
					frame := water(0, 0, 0)
					if nextFrame >= splash && nextFrame < splash+600*time.Millisecond {
						frame = water(50, 45, 10)
					}
					if d.Feed(frame, start.Add(nextFrame)) {
						t.Fatalf("fired on the splash alone at %v", nextFrame)
					}
				}
				if heard := at - lag; heard >= 0 && d.Hear(g.packet(heard), start.Add(heard)) {
					return
				}
			}
			t.Fatal("missed the bite sound that started during the splash")
		})
	}
}

// pond is a w by h box of water with a little noise on every frame, the way
// the game's water shimmers. splash brightens a size by size patch at the
// centre, as a far bobber's splash does; shift adds to every pixel, as a camera
// turn or a change of light does.
func pond(rng *rand.Rand, w, h, splash int, shift byte) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			v := 60 + byte(rng.IntN(4)) + shift
			if splash > 0 && abs(x-w/2) < splash/2 && abs(y-h/2) < splash/2 {
				v = 200
			}
			i := img.PixOffset(x, y)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = v, v+10, v+20, 255
		}
	}
	return img
}

// watch feeds one frame every 33 ms for the given time, the app's frame rate,
// and returns when the detector fired.
func watch(d BiteDetector, length time.Duration, frame func(at time.Duration) *image.RGBA) (time.Duration, bool) {
	start := time.Unix(0, 0)
	for at := time.Duration(0); at < length; at += 33 * time.Millisecond {
		if d.Feed(frame(at), start.Add(at)) {
			return at, true
		}
	}
	return 0, false
}

// With the game's sound off, a splash at the centre is enough.
func TestSplashSoundWithoutGameSound(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	at, fired := watch(NewSplashSoundDetector(nil), 7*time.Second, func(at time.Duration) *image.RGBA {
		if at >= 4*time.Second {
			return pond(rng, 100, 90, 30, 0)
		}
		return pond(rng, 100, 90, 0, 0)
	})
	if !fired || at < 4*time.Second || at > 4200*time.Millisecond {
		t.Fatalf("fired = %v at %v, want the splash at 4s", fired, at)
	}
}

// disc returns calm water with a disc of colour c at the centre of the box.
func disc(c color.RGBA) *image.RGBA {
	img := water(0, 0, 0)
	for py := range 90 {
		for px := range 100 {
			if (px-50)*(px-50)+(py-45)*(py-45) <= 12*12 {
				img.SetRGBA(px, py, c)
			}
		}
	}
	return img
}

// Only white spray at the bobber is a splash. A pet or a player passing
// through the box is dark, a spell effect coloured: neither counts, even with
// a bite sound at the right time.
func TestSplashSoundNeedsWhiteSprayAtTheBobber(t *testing.T) {
	tests := []struct {
		name  string
		frame *image.RGBA
		want  bool
	}{
		{"white spray", disc(color.RGBA{190, 200, 205, 255}), true},
		{"a dark pet swimming past", disc(color.RGBA{20, 25, 20, 255}), false},
		{"a red spell effect", disc(color.RGBA{230, 60, 50, 255}), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewSplashSoundDetector(nil)
			start := time.Unix(0, 0)
			quiet, loud := tone(5), tone(200)
			fired := false
			for at := time.Duration(0); at < 6*time.Second && !fired; at += 10 * time.Millisecond {
				if at%(100*time.Millisecond) == 0 {
					frame := water(0, 0, 0)
					if at >= 4*time.Second && at < 4500*time.Millisecond {
						frame = tt.frame
					}
					fired = d.Feed(frame, start.Add(at))
				}
				packet := quiet
				if at >= 4300*time.Millisecond && at < 4600*time.Millisecond {
					packet = loud
				}
				fired = fired || d.Hear(packet, start.Add(at))
			}
			if fired != tt.want {
				t.Errorf("fired = %v, want %v", fired, tt.want)
			}
		})
	}
}
