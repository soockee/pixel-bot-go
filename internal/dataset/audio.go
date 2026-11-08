package dataset

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/soocke/pixel-bot-go/domain/capture"
)

const (
	audioFile       = "audio.wav"
	wavHeaderBytes  = 44
	audioGapSamples = capture.AudioRate / 50 // a packet more than 20 ms late leaves a gap
)

// AudioPath returns the sound file of a dataset.
func AudioPath(dir string) string { return filepath.Join(dir, audioFile) }

// AudioWriter saves sound as audio.wav. Gaps are filled with silence, so sample
// n plays n/capture.AudioRate seconds after start, on the clock of the frame
// names. A killed recording leaves the header sizes at zero; the samples are
// still everything after the 44-byte header.
type AudioWriter struct {
	file    *os.File
	start   time.Time
	samples int64
}

// CreateAudio starts audio.wav in a directory from Create.
func CreateAudio(dir string, start time.Time) (*AudioWriter, error) {
	file, err := os.Create(AudioPath(dir))
	if err != nil {
		return nil, err
	}
	if _, err := file.Write(wavHeader(0)); err != nil {
		file.Close()
		return nil, err
	}
	return &AudioWriter{file: file, start: start}, nil
}

// Write appends a packet of capture.CaptureAudio that played at the given time.
func (w *AudioWriter) Write(pcm []byte, at time.Time) error {
	due := int64(at.Sub(w.start) * capture.AudioRate / time.Second)
	if due-w.samples > audioGapSamples {
		silence := make([]byte, capture.AudioRate*capture.AudioFrameBytes)
		for w.samples < due {
			n := min(due-w.samples, capture.AudioRate)
			if _, err := w.file.Write(silence[:n*capture.AudioFrameBytes]); err != nil {
				return err
			}
			w.samples += n
		}
	}
	n, err := w.file.Write(pcm)
	w.samples += int64(n / capture.AudioFrameBytes)
	return err
}

// Audio returns the samples of audio.wav in the layout of capture.CaptureAudio,
// or nil when the dataset has no sound.
func (d *Dataset) Audio() ([]byte, error) {
	data, err := os.ReadFile(AudioPath(d.Dir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// Sizes are ignored, because a killed recording leaves them at zero.
	if len(data) < wavHeaderBytes || !bytes.Equal(data[8:36], wavHeader(0)[8:36]) {
		return nil, fmt.Errorf("%s is not %d Hz %d-bit stereo PCM", audioFile, capture.AudioRate, capture.AudioBits)
	}
	return data[wavHeaderBytes:], nil
}

// Close writes the final sizes into the header.
func (w *AudioWriter) Close() error {
	_, err := w.file.WriteAt(wavHeader(uint32(w.samples*capture.AudioFrameBytes)), 0)
	if closeErr := w.file.Close(); err == nil {
		err = closeErr
	}
	return err
}

func wavHeader(dataBytes uint32) []byte {
	h := make([]byte, 0, wavHeaderBytes)
	le := binary.LittleEndian
	h = append(h, "RIFF"...)
	h = le.AppendUint32(h, 36+dataBytes)
	h = append(h, "WAVEfmt "...)
	h = le.AppendUint32(h, 16)
	h = le.AppendUint16(h, 1) // PCM
	h = le.AppendUint16(h, capture.AudioChannels)
	h = le.AppendUint32(h, capture.AudioRate)
	h = le.AppendUint32(h, capture.AudioRate*capture.AudioFrameBytes)
	h = le.AppendUint16(h, capture.AudioFrameBytes)
	h = le.AppendUint16(h, capture.AudioBits)
	h = append(h, "data"...)
	return le.AppendUint32(h, dataBytes)
}
