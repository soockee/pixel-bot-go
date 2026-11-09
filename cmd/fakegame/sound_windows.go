package main

import (
	"encoding/binary"
	"math"
	"math/rand"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/soocke/pixel-bot-go/domain/capture"
)

var playSound = windows.NewLazySystemDLL("winmm.dll").NewProc("PlaySoundW")

const (
	sndAsync     = 0x0001
	sndNoDefault = 0x0002
	sndMemory    = 0x0004
	sndLoop      = 0x0008
	humLevel     = 9 // RMS of 16-bit samples; recorded fishing has a median of 6
)

// speaker plays the scenario's sound from this process, the way the game
// plays its own, so the app's process loopback hears it. It plays one sound
// at a time: the hum loops, and a bite plays mixed into the hum, then the hum
// loops again.
type speaker struct {
	mu   sync.Mutex
	hum  []byte // the hum as a WAV file, looped
	hums []byte // the hum's samples, to mix into bites
	wavs map[string][]byte
	play []byte // the WAV playing; PlaySound reads it while it plays
	gen  int    // counts bites, so an old bite's end does not restart the hum
}

// newSpeaker starts the hum and prepares the bite sounds, given as
// capture.CaptureAudio PCM by name.
func newSpeaker(bites map[string][]byte, seed int64) *speaker {
	rng := rand.New(rand.NewSource(seed))
	hum := make([]byte, 2*capture.AudioRate*capture.AudioFrameBytes) // 2 s
	for i := 0; i < len(hum); i += 2 {
		binary.LittleEndian.PutUint16(hum[i:], uint16(int16(rng.NormFloat64()*humLevel)))
	}
	s := &speaker{hum: wav(hum), hums: hum, wavs: map[string][]byte{}}
	for name, pcm := range bites {
		mixed := make([]byte, len(pcm))
		for i := 0; i+1 < len(pcm); i += 2 {
			v := int(int16(binary.LittleEndian.Uint16(pcm[i:]))) + int(int16(binary.LittleEndian.Uint16(hum[i%len(hum):])))
			binary.LittleEndian.PutUint16(mixed[i:], uint16(int16(max(math.MinInt16, min(math.MaxInt16, v)))))
		}
		s.wavs[name] = wav(mixed)
	}
	s.mu.Lock()
	s.start(s.hum, sndLoop)
	s.mu.Unlock()
	return s
}

// bite plays the named bite sound, then goes back to the hum.
func (s *speaker) bite(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := s.wavs[name]
	s.gen++
	gen := s.gen
	s.start(w, 0)
	length := time.Duration(len(w)-44) * time.Second / (capture.AudioRate * capture.AudioFrameBytes)
	time.AfterFunc(length, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.gen == gen {
			s.start(s.hum, sndLoop)
		}
	})
}

// start plays w in place of the current sound. The caller holds mu.
func (s *speaker) start(w []byte, flags uintptr) {
	s.play = w
	_, _, _ = playSound.Call(uintptr(unsafe.Pointer(&w[0])), 0, sndMemory|sndAsync|sndNoDefault|flags)
}

// wav wraps capture.CaptureAudio PCM in a WAV header.
func wav(pcm []byte) []byte {
	h := make([]byte, 44, 44+len(pcm))
	copy(h, "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+len(pcm)))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1) // PCM
	binary.LittleEndian.PutUint16(h[22:], capture.AudioChannels)
	binary.LittleEndian.PutUint32(h[24:], capture.AudioRate)
	binary.LittleEndian.PutUint32(h[28:], capture.AudioRate*capture.AudioFrameBytes)
	binary.LittleEndian.PutUint16(h[32:], capture.AudioFrameBytes)
	binary.LittleEndian.PutUint16(h[34:], capture.AudioBits)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(len(pcm)))
	return append(h, pcm...)
}
