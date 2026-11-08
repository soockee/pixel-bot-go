package dataset

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/domain/capture"
)

func TestAudioKeepsPacketsOnTheFrameClock(t *testing.T) {
	dir := t.TempDir()
	start := time.Unix(100, 0)
	w, err := CreateAudio(dir, start)
	if err != nil {
		t.Fatal(err)
	}
	packet := bytes.Repeat([]byte{1, 0, 2, 0}, 441) // 10 ms
	writes := []time.Duration{0, 12 * time.Millisecond, 1500 * time.Millisecond}
	for _, after := range writes {
		if err := w.Write(packet, start.Add(after)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(AudioPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	pcm := data[wavHeaderBytes:]
	le := binary.LittleEndian
	if string(data[:4]) != "RIFF" || le.Uint32(data[4:]) != uint32(len(data)-8) || le.Uint32(data[40:]) != uint32(len(pcm)) {
		t.Fatalf("header sizes do not match %d bytes", len(data))
	}
	if got := le.Uint32(data[24:]); got != capture.AudioRate {
		t.Errorf("rate = %d", got)
	}
	// 2 ms late stays joined; 1.5 s late starts after silence, exactly on time.
	at := func(d time.Duration) int { return int(d*capture.AudioRate/time.Second) * capture.AudioFrameBytes }
	if pcm[at(10*time.Millisecond)] != 1 || pcm[at(30*time.Millisecond)] != 0 || pcm[at(1500*time.Millisecond)] != 1 {
		t.Error("packets are not where they played")
	}
	if want := at(1510 * time.Millisecond); len(pcm) != want {
		t.Errorf("pcm = %d bytes, want %d", len(pcm), want)
	}
}
