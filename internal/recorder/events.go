package recorder

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

// EventPageSize is the most records ReadEvents returns at once.
const EventPageSize = 512

// EventPage is one page of trace records, read from Offset on.
type EventPage struct {
	Events     []json.RawMessage `json:"events"`
	NextOffset int64             `json:"nextOffset"`
	More       bool              `json:"more"` // another full page follows
}

// ErrInvalidOffset reports an offset that is not the start of a record.
var ErrInvalidOffset = errors.New("offset must point to the start of a record")

// ReadEvents reads the complete trace records of the file at path from offset
// on. A writer can be in the middle of the last line, so a partial record is
// left for the next read.
func ReadEvents(path string, offset int64) (EventPage, error) {
	page := EventPage{Events: []json.RawMessage{}, NextOffset: offset}
	trace, err := os.Open(path)
	if err != nil {
		return page, err
	}
	defer trace.Close()
	info, err := trace.Stat()
	if err != nil {
		return page, fmt.Errorf("read trace metadata: %w", err)
	}
	if !info.Mode().IsRegular() {
		return page, errors.New("trace must be a regular file")
	}
	size := info.Size()
	if offset < 0 || offset > size {
		return page, ErrInvalidOffset
	}
	if offset > 0 {
		var previous [1]byte
		if _, err := trace.ReadAt(previous[:], offset-1); err != nil {
			return page, fmt.Errorf("read trace offset: %w", err)
		}
		if previous[0] != '\n' {
			return page, ErrInvalidOffset
		}
	}
	reader := bufio.NewReaderSize(io.NewSectionReader(trace, offset, size-offset), 4096)
	for len(page.Events) < EventPageSize {
		line, err := reader.ReadSlice('\n')
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return page, fmt.Errorf("read trace record: %w", err)
		}
		value := bytes.TrimSpace(line)
		if !json.Valid(value) || value[0] != '{' {
			return page, fmt.Errorf("invalid JSON record at byte %d", page.NextOffset)
		}
		page.Events = append(page.Events, json.RawMessage(bytes.Clone(value)))
		page.NextOffset += int64(len(line))
	}
	page.More = len(page.Events) == EventPageSize && page.NextOffset < size
	return page, nil
}
