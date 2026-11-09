package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/soocke/pixel-bot-go/internal/playtrace"
)

// SensitiveSpan marks a typing burst without displaying its contents.
type SensitiveSpan struct {
	StartUS int64 `json:"startUs"`
	EndUS   int64 `json:"endUs"`
	Presses int   `json:"presses"`
}

// RecordingReview describes what is available and what the upload will omit.
type RecordingReview struct {
	Fingerprint    string          `json:"fingerprint"`
	Frames         int             `json:"frames"`
	FrameNames     []string        `json:"frameNames"`
	HasAudio       bool            `json:"hasAudio"`
	HasInput       bool            `json:"hasInput"`
	KeyEvents      int             `json:"keyEvents"`
	Sensitive      []SensitiveSpan `json:"sensitive"`
	SensitiveCount int             `json:"sensitiveCount"`
}

// ReviewRecording reads local files without contacting the upload service.
func ReviewRecording(dir string) (RecordingReview, error) {
	review, _, err := reviewRecording(dir, io.Discard)
	return review, err
}

func reviewRecording(dir string, sanitized io.Writer) (RecordingReview, []file, error) {
	var review RecordingReview
	files, err := recordingFiles(dir)
	if err != nil {
		return review, nil, err
	}
	slices.SortFunc(files, func(a, b file) int { return strings.Compare(a.Path, b.Path) })
	digest := sha256.New()
	fmt.Fprintln(digest, filepath.Base(dir))
	for _, f := range files {
		info, err := os.Lstat(f.local)
		if err != nil || !info.Mode().IsRegular() || info.Size() != f.Size {
			return review, nil, errors.New("recording changed during review; reopen the review")
		}
		fmt.Fprintln(digest, f.Path, f.Size, info.ModTime().UnixNano())
		switch {
		case strings.HasPrefix(f.Path, "frames/"):
			review.Frames++
			review.FrameNames = append(review.FrameNames, filepath.Base(f.Path))
		case f.Path == "audio.wav":
			review.HasAudio = true
		case f.Path == "input.jsonl":
			input, err := os.Open(f.local)
			if err != nil {
				return review, nil, err
			}
			err = sanitizeTrace(io.TeeReader(input, digest), sanitized, &review)
			closeErr := input.Close()
			if err = errors.Join(err, closeErr); err != nil {
				return review, nil, fmt.Errorf("review input trace: %w", err)
			}
			review.HasInput = true
		}
	}
	review.Fingerprint = fmt.Sprintf("%x", digest.Sum(nil))
	return review, files, nil
}

// UploadReviewed uploads a separate, key-free copy of the selected media.
func (s *Session) UploadReviewed(ctx context.Context, dir, fingerprint string, frames, audio bool, progress func(done, total int)) error {
	if fingerprint == "" {
		return errors.New("review the recording before uploading")
	}
	out, err := os.CreateTemp("", "gofish-reviewed-input-*.jsonl")
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	review, files, err := reviewRecording(dir, out)
	if err = errors.Join(err, out.Close()); err != nil {
		return err
	}
	if review.Fingerprint != fingerprint {
		return errors.New("recording changed; reopen the review before uploading")
	}
	if !review.HasInput && !(frames && review.Frames > 0) && !(audio && review.HasAudio) {
		return errors.New("select at least one recording component to upload")
	}
	files = slices.DeleteFunc(files, func(f file) bool {
		return !frames && strings.HasPrefix(f.Path, "frames/") || !audio && f.Path == "audio.wav"
	})
	for i := range files {
		if files[i].Path == "input.jsonl" {
			stat, err := os.Stat(out.Name())
			if err != nil {
				return err
			}
			files[i].local, files[i].Size = out.Name(), stat.Size()
		}
	}
	// Earlier uploads are immutable. Use a separate ID for the sanitized copy.
	id := fmt.Sprintf("reviewed-%s-%t-%t", review.Fingerprint[:32], frames, audio)
	return s.uploadFiles(ctx, id, files, progress)
}

func sanitizeTrace(input io.Reader, output io.Writer, review *RecordingReview) error {
	dec := json.NewDecoder(input)
	enc := json.NewEncoder(output)
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return err
	}
	header, err := playtrace.DecodeHeader(raw)
	if err != nil {
		return err
	}
	header.Keyboard = false
	header.SessionID = ""
	if err := enc.Encode(header); err != nil {
		return err
	}
	var span SensitiveSpan
	var segment int
	endSeen := false
	finishSpan := func() {
		if span.Presses >= 6 {
			review.SensitiveCount++
			if len(review.Sensitive) < 30 {
				review.Sensitive = append(review.Sensitive, span)
			}
		}
		span = SensitiveSpan{}
	}
	for {
		if err := dec.Decode(&raw); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return err
		}
		var event playtrace.Event
		if err := json.Unmarshal(raw, &event); err != nil {
			return err
		}
		if endSeen {
			return errors.New("unexpected events after the end of the recording")
		}
		switch event.Type {
		case "key_down", "key_up":
			review.KeyEvents++
			if event.Type == "key_down" && event.Key != nil && !event.Key.Repeat &&
				possibleTypingKey(*event.Key) {
				if span.Presses != 0 && (event.Segment != segment || event.ElapsedUS-span.EndUS > 750_000) {
					finishSpan()
				}
				if span.Presses == 0 {
					span.StartUS, segment = event.ElapsedUS, event.Segment
				}
				span.Presses++
				span.EndUS = event.ElapsedUS
			}
		case "segment_start", "segment_end", "move", "click", "drag", "button_down", "button_up", "wheel":
			if event.Type == "segment_end" {
				finishSpan()
			}
			if err := enc.Encode(playtrace.Event{
				Type: event.Type, ElapsedUS: event.ElapsedUS, Segment: event.Segment,
				Button: event.Button, Position: event.Position, DX: event.DX, DY: event.DY,
				Wheel: event.Wheel, Gesture: event.Gesture,
			}); err != nil {
				return err
			}
		case "session_end":
			var end playtrace.End
			if err := json.Unmarshal(raw, &end); err != nil {
				return err
			}
			end.KeyEvents = 0
			if end.Status != "complete" {
				end.Status = "incomplete"
			}
			if err := enc.Encode(end); err != nil {
				return err
			}
			endSeen = true
		}
	}
	finishSpan()
	if !endSeen {
		return errors.New("input trace is unfinished")
	}
	return nil
}

func possibleTypingKey(key playtrace.Key) bool {
	vk := key.VK
	return len(key.Name) == 1 || vk == 0x08 || vk == 0x20 ||
		vk >= 0x30 && vk <= 0x39 || vk >= 0x41 && vk <= 0x5a ||
		vk >= 0x60 && vk <= 0x6f || vk >= 0xba && vk <= 0xc0 ||
		vk >= 0xdb && vk <= 0xdf || vk == 0xe2
}
