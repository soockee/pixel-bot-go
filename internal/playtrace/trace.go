// Package playtrace defines the JSONL interface for fishing playtest traces.
package playtrace

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	Format = "fishing-trace"
	Schema = 3

	Manual    = "manual"
	FakeGame  = "fake_game"
	AppDryRun = "app_dry_run"
	Benchmark = "responsebench"
	Monotonic = "monotonic"
	Virtual   = "virtual"

	BiteAvailable  = "bite_available"
	BiteDetected   = "bite_detected"
	CastRequested  = "cast_requested"
	CastDispatched = "cast_dispatched"
	ReelDispatched = "reel_dispatched"
	ReelPlanned    = "reel_planned"
	ReelReleased   = "reel_released"
	ReelCanceled   = "reel_canceled"
	TimingGap      = "timing_gap"
	ReelAttempt    = "reel_attempt"
	ReelReply      = "reel_reply"
	BiteClosed     = "bite_closed"
)

// Header identifies the producer and clock. A shared SessionID explicitly
// opts wall-clock streams into alignment. Benchmark runs use virtual time.
type Header struct {
	Type          string     `json:"type"`
	Format        string     `json:"format"`
	Schema        int        `json:"schema"`
	Source        string     `json:"source"`
	Clock         string     `json:"clock"`
	SessionID     string     `json:"session_id,omitempty"`
	StartedUTC    *time.Time `json:"started_utc,omitempty"`
	Keyboard      bool       `json:"keyboard,omitempty"`
	DragPixels    int        `json:"drag_pixels,omitempty"`
	DragCounts    int        `json:"drag_counts,omitempty"`
	TimingProfile string     `json:"timing_profile,omitempty"`
	TimingSeed    *uint64    `json:"timing_seed,omitempty"`
	CursorProfile string     `json:"cursor_profile,omitempty"`
}

// Event is one observation. ElapsedUS is relative to the header anchor for
// wall-clock files and restarts for each RunID in benchmark files.
type Event struct {
	Type       string      `json:"type"`
	ElapsedUS  int64       `json:"t_us"`
	Segment    int         `json:"segment,omitempty"`
	EventID    *int        `json:"event_id,omitempty"`
	RunID      string      `json:"run_id,omitempty"`
	ScenarioID string      `json:"scenario_id,omitempty"`
	Seed       json.Number `json:"seed,omitempty"`
	Controller string      `json:"controller,omitempty"`
	Attempt    int         `json:"attempt,omitempty"`
	Outcome    string      `json:"outcome,omitempty"`
	Phase      string      `json:"phase,omitempty"`
	Button     string      `json:"button,omitempty"`
	Position   *Position   `json:"position,omitempty"`
	DX         int32       `json:"dx,omitempty"`
	DY         int32       `json:"dy,omitempty"`
	Wheel      int16       `json:"wheel,omitempty"`
	Key        *Key        `json:"key,omitempty"`
	Gesture    *Gesture    `json:"gesture,omitempty"`
	Status     string      `json:"status,omitempty"`
	Timing     *ReelTiming `json:"timing,omitempty"`
}

// ReelTiming records the detector timestamp and plan for a simulated reel.
type ReelTiming struct {
	Detector   string `json:"detector,omitempty"`
	DetectorUS int64  `json:"detector_us"`
	DelayUS    int64  `json:"delay_us"`
	HoldUS     int64  `json:"hold_us"`
	CooldownUS int64  `json:"cooldown_us"`
}

// Position uses client pixels and includes dimensions for normalization.
type Position struct {
	X      int32 `json:"x"`
	Y      int32 `json:"y"`
	Width  int32 `json:"width"`
	Height int32 `json:"height"`
}

// Key is one keyboard transition without character translation.
type Key struct {
	Name     string `json:"name"`
	VK       uint32 `json:"vk"`
	Scan     uint32 `json:"scan"`
	Extended bool   `json:"extended,omitempty"`
	Repeat   bool   `json:"repeat,omitempty"`
	Chord    string `json:"chord,omitempty"`
}

// Gesture summarizes a hold in client pixels and raw mouse counts.
type Gesture struct {
	DurationUS      int64    `json:"duration_us"`
	Start           Position `json:"start"`
	End             Position `json:"end"`
	MaxClientPixels float64  `json:"max_client_pixels"`
	RawDX           int64    `json:"raw_dx"`
	RawDY           int64    `json:"raw_dy"`
	RawTravel       float64  `json:"raw_travel"`
	MaxRawCounts    float64  `json:"max_raw_counts"`
}

// End finalizes a trace. Input counts are populated by the manual recorder.
type End struct {
	Type         string `json:"type"`
	ElapsedUS    int64  `json:"t_us"`
	Moves        int    `json:"moves,omitempty"`
	ButtonEvents int    `json:"button_events,omitempty"`
	KeyEvents    int    `json:"key_events,omitempty"`
	Clicks       int    `json:"clicks,omitempty"`
	Drags        int    `json:"drags,omitempty"`
	Segments     int    `json:"segments,omitempty"`
	Status       string `json:"status"`
}

// Writer owns a new trace file. Calls are serialized and the first write
// error is returned again at Close. Producers supply elapsed monotonic times.
type Writer struct {
	mu      sync.Mutex
	file    *os.File
	buffer  *bufio.Writer
	encoder *json.Encoder
	err     error
	closed  bool
}

// Create refuses to overwrite an existing trace.
func Create(path string, header Header) (*Writer, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	buffer := bufio.NewWriterSize(file, 64*1024)
	w := &Writer{file: file, buffer: buffer, encoder: json.NewEncoder(buffer)}
	header.Type, header.Format, header.Schema = "session_start", Format, Schema
	if err := w.encoder.Encode(header); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	if err := w.Flush(); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return w, nil
}

// Write appends one event. Callers must submit events in order per run.
func (w *Writer) Write(event Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return os.ErrClosed
	}
	if w.err == nil {
		w.err = w.encoder.Encode(event)
	}
	return w.err
}

// Flush makes buffered rows available to readers of a live trace.
func (w *Writer) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return os.ErrClosed
	}
	if w.err == nil {
		w.err = w.buffer.Flush()
	}
	return w.err
}

// Close appends the footer and closes the file once.
func (w *Writer) Close(end End) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return w.err
	}
	w.closed = true
	end.Type = "session_end"
	if w.err == nil {
		w.err = w.encoder.Encode(end)
	}
	if w.err == nil {
		w.err = w.buffer.Flush()
	}
	w.err = errors.Join(w.err, w.file.Close())
	return w.err
}

// DecodeHeader accepts the current format and legacy recorder schemas 1 and 2.
func DecodeHeader(raw json.RawMessage) (Header, error) {
	var h Header
	if err := json.Unmarshal(raw, &h); err != nil {
		return h, err
	}
	if h.Type != "session_start" {
		return h, errors.New("missing session header")
	}
	if h.Format == "" && (h.Schema == 1 || h.Schema == 2) {
		h.Source, h.Clock = Manual, Monotonic
		return h, nil
	}
	if h.Format != Format || h.Schema != Schema {
		return h, fmt.Errorf("unsupported trace format %q schema %d", h.Format, h.Schema)
	}
	switch h.Source {
	case Manual, FakeGame, AppDryRun:
		if h.Clock == Monotonic && h.StartedUTC != nil && !h.StartedUTC.IsZero() {
			return h, nil
		}
	case Benchmark:
		if h.Clock == Virtual {
			return h, nil
		}
	}
	return h, fmt.Errorf("invalid trace source or clock for %q", h.Source)
}

// ReadEvents decodes an ordered stream after its header and reports malformed rows.
func ReadEvents(decoder *json.Decoder, visit func(Event) error) error {
	for {
		var event Event
		if err := decoder.Decode(&event); errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return err
		}
		if event.Type == "" || event.ElapsedUS < 0 {
			return errors.New("invalid trace event type or timestamp")
		}
		if err := visit(event); err != nil {
			return err
		}
	}
}
