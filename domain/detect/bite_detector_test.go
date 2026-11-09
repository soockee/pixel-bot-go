package detect

import (
	"image"
	"math/rand/v2"
	"testing"
	"time"
)

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

func TestMotion(t *testing.T) {
	const s = time.Second
	tests := []struct {
		name string
		w, h int
		// frame returns the splash size and the light shift at a time.
		frame func(at time.Duration) (splash int, shift byte)
		want  bool
	}{
		{"calm water", 100, 90, func(time.Duration) (int, byte) { return 0, 0 }, false},
		{"close bobber bites", 100, 90, func(at time.Duration) (int, byte) {
			if at >= 4*s {
				return 30, 0
			}
			return 0, 0
		}, true},
		{"far bobber bites: a 6 px splash in a large box", 160, 120, func(at time.Duration) (int, byte) {
			if at >= 4*s {
				return 6, 0
			}
			return 0, 0
		}, true},
		{"the landing splash", 100, 90, func(at time.Duration) (int, byte) {
			if at < time.Second && at/(100*time.Millisecond)%2 == 0 {
				return 40, 0
			}
			return 0, 0
		}, false},
		{"camera turn: everything changes", 100, 90, func(at time.Duration) (int, byte) {
			if at >= 4*s {
				return 0, byte(at/(33*time.Millisecond)%3) * 20
			}
			return 0, 0
		}, false},
		{"slow light drift", 100, 90, func(at time.Duration) (int, byte) { return 0, byte(at / (200 * time.Millisecond)) }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rng := rand.New(rand.NewPCG(1, 2))
			at, fired := watch(NewMotionBiteDetector(nil), 7*s, func(at time.Duration) *image.RGBA {
				splash, shift := tt.frame(at)
				return pond(rng, tt.w, tt.h, splash, shift)
			})
			if fired != tt.want {
				t.Fatalf("fired = %v at %v, want %v", fired, at, tt.want)
			}
			if fired && (at < 4*s || at > 4*s+200*time.Millisecond) {
				t.Errorf("fired at %v, want within 200 ms of the bite at 4s", at)
			}
		})
	}
}

func TestMotionFiresOnceUntilReset(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	bite := func(at time.Duration) *image.RGBA {
		if at >= 3*time.Second {
			return pond(rng, 100, 90, 30, 0)
		}
		return pond(rng, 100, 90, 0, 0)
	}
	d := NewMotionBiteDetector(nil)
	if _, fired := watch(d, 5*time.Second, bite); !fired {
		t.Fatal("no bite")
	}
	if _, fired := watch(d, 5*time.Second, bite); fired {
		t.Fatal("fired twice without a Reset")
	}
	d.Reset()
	if _, fired := watch(d, 5*time.Second, bite); !fired {
		t.Fatal("no bite after Reset")
	}
}

// A far bobber's small splash is too small for the bright-splash rule, but the
// change at the centre counts as the splash before the bite sound.
func TestSplashSoundHearsAFarBobber(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	d := NewSplashSoundDetector(nil)
	start := time.Unix(0, 0)
	quiet, loud := tone(5), tone(200)
	for at := time.Duration(0); at < 7*time.Second; at += 10 * time.Millisecond {
		if at%(30*time.Millisecond) == 0 {
			splash := 0
			if at >= 4*time.Second {
				splash = 6
			}
			if d.Feed(pond(rng, 160, 120, splash, 0), start.Add(at)) {
				t.Fatalf("fired on the splash alone at %v while the game made sound", at)
			}
		}
		packet := quiet
		if at >= 4300*time.Millisecond && at < 4600*time.Millisecond {
			packet = loud
		}
		if d.Hear(packet, start.Add(at)) {
			if at < 4300*time.Millisecond || at > 4350*time.Millisecond {
				t.Fatalf("fired at %v, want with the sound at 4.3s", at)
			}
			return
		}
	}
	t.Fatal("missed the far bobber's bite")
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
