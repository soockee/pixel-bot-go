package detect

import (
	"encoding/binary"
	"testing"
	"time"

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
