package dataset

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/soocke/pixel-bot-go/internal/playtrace"
)

const (
	sessionFile = "session.json"
	landingMS   = 2000 // the bobber is in the air for this long after a cast
	settleMS    = 1000 // the bobber is gone this long after a reel
	minEmptyMS  = 1000 // shorter gaps are not worth measuring
)

// SessionInfo holds what labels need beyond the frames and the input trace.
// shortcut: the offset is read once, so moving the window mid-session makes the
// bobber boxes wrong; measure it per click if that turns out to matter.
type SessionInfo struct {
	ClientToFrame image.Point `json:"client_to_frame"` // add to a client position to get frame pixels
}

// WriteSessionInfo saves s into a dataset directory.
func WriteSessionInfo(dir string, s SessionInfo) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, sessionFile), data, 0o644)
}

// AutoLabelOptions says how to read a session's input.
type AutoLabelOptions struct {
	Cast       string // binding that casts, for example mouse:x1 or key:F3
	Reel       string // binding that reels, for example mouse:right
	ReactionMS int64  // assumed delay from the bite to the reel click
	BoxPx      int    // side of the bobber box around the reel click
}

type binding struct{ kind, value string }

func parseBinding(s string) (binding, error) {
	kind, value, ok := strings.Cut(s, ":")
	if !ok || value == "" || (kind != "mouse" && kind != "key") {
		return binding{}, fmt.Errorf("binding %q must look like mouse:x1 or key:F3", s)
	}
	return binding{kind, value}, nil
}

func (b binding) matches(e playtrace.Event) bool {
	if b.kind == "mouse" {
		return e.Type == "click" && e.Button == b.value
	}
	if e.Type != "key_down" || e.Key == nil || e.Key.Repeat {
		return false
	}
	name := e.Key.Chord
	if name == "" {
		name = e.Key.Name
	}
	return strings.EqualFold(name, b.value)
}

// draggedBy reports a press that moved. That still casts, though a right button
// drag only turns the camera, so reels do not use this.
func (b binding) draggedBy(e playtrace.Event) bool {
	return b.kind == "mouse" && e.Type == "drag" && e.Button == b.value
}

// AutoLabel builds labels from a session's input. A cast with no reel has no
// known bobber position, so it is left out and described in the skipped list.
func AutoLabel(dir string, opts AutoLabelOptions) (Labels, []string, error) {
	var labels Labels
	cast, err := parseBinding(opts.Cast)
	if err != nil {
		return labels, nil, err
	}
	reel, err := parseBinding(opts.Reel)
	if err != nil {
		return labels, nil, err
	}
	info, bounds, err := loadSession(dir)
	if err != nil {
		return labels, nil, err
	}
	var skipped []string
	var pending *int64  // time of the cast waiting for its reel
	var clearFrom int64 // the frame is free of bobbers from here until a cast
	err = readInput(dir, func(e playtrace.Event) error {
		ms := e.ElapsedUS / 1000
		switch {
		case cast.matches(e) || cast.draggedBy(e):
			if pending != nil {
				skipped = append(skipped, fmt.Sprintf("cast at %d ms has no reel", *pending))
			} else if ms-clearFrom >= minEmptyMS {
				labels.Empty = append(labels.Empty, Span{clearFrom, ms})
			}
			pending = &ms
		case reel.matches(e) && pending != nil:
			c, problem := labelCast(*pending, ms, e.Position, info, bounds, opts)
			if problem != "" {
				skipped = append(skipped, problem)
			} else {
				labels.Casts = append(labels.Casts, c)
			}
			pending, clearFrom = nil, ms+settleMS
		}
		return nil
	})
	if err != nil {
		return labels, skipped, err
	}
	if pending != nil {
		skipped = append(skipped, fmt.Sprintf("cast at %d ms has no reel", *pending))
	}
	if len(labels.Casts) == 0 {
		return labels, skipped, fmt.Errorf("no cast and reel pairs found; check the bindings %s and %s", opts.Cast, opts.Reel)
	}
	return labels, skipped, labels.check()
}

func labelCast(castMS, reelMS int64, click *playtrace.Position, info SessionInfo, bounds image.Rectangle, opts AutoLabelOptions) (Cast, string) {
	start, bite := castMS+landingMS, reelMS-opts.ReactionMS
	if click == nil {
		return Cast{}, fmt.Sprintf("reel at %d ms has no position", reelMS)
	}
	if bite <= start {
		return Cast{}, fmt.Sprintf("reel at %d ms came too soon after its cast", reelMS)
	}
	at := image.Pt(int(click.X), int(click.Y)).Add(info.ClientToFrame)
	if !at.In(bounds) {
		return Cast{}, fmt.Sprintf("reel at %d ms clicked %v, outside the frame", reelMS, at)
	}
	half := opts.BoxPx / 2
	box := image.Rect(at.X-half, at.Y-half, at.X+half, at.Y+half).Intersect(bounds)
	return Cast{
		Span:   Span{start, reelMS},
		Bobber: Rect{box.Min.X, box.Min.Y, box.Dx(), box.Dy()},
		BiteMS: &bite,
	}, ""
}

// loadSession reads the session info and the frame bounds.
func loadSession(dir string) (SessionInfo, image.Rectangle, error) {
	var info SessionInfo
	data, err := os.ReadFile(filepath.Join(dir, sessionFile))
	if err != nil {
		return info, image.Rectangle{}, fmt.Errorf("record with -window to get labels: %w", err)
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return info, image.Rectangle{}, fmt.Errorf("%s: %w", sessionFile, err)
	}
	path := LatestFrame(FramesPath(dir))
	if path == "" {
		return info, image.Rectangle{}, errors.New("session has no frames")
	}
	file, err := os.Open(path)
	if err != nil {
		return info, image.Rectangle{}, err
	}
	defer file.Close()
	cfg, err := png.DecodeConfig(file)
	if err != nil {
		return info, image.Rectangle{}, fmt.Errorf("%s: %w", path, err)
	}
	return info, image.Rect(0, 0, cfg.Width, cfg.Height), nil
}

func readInput(dir string, visit func(playtrace.Event) error) error {
	file, err := os.Open(InputPath(dir))
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	var first json.RawMessage
	if err := decoder.Decode(&first); err != nil {
		return fmt.Errorf("%s: %w", inputFile, err)
	}
	if _, err := playtrace.DecodeHeader(first); err != nil {
		return fmt.Errorf("%s: %w", inputFile, err)
	}
	// A recording that was killed can end in half a line; keep what came before.
	if err := playtrace.ReadEvents(decoder, visit); err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return fmt.Errorf("%s: %w", inputFile, err)
	}
	return nil
}

// WriteLabels saves labels as labels.json. It refuses to replace an existing
// file, which may hold hand corrections.
func WriteLabels(dir string, labels Labels) error {
	data, err := json.MarshalIndent(labels, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(dir, labelsFile), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
