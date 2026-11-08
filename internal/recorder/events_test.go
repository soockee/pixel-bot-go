package recorder

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTrace(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadEventsReadsOnlyCompleteRecords(t *testing.T) {
	const record = "{\"type\":\"move\"}\n"
	path := writeTrace(t, record+"{\"type\":")
	page, err := ReadEvents(path, 0)
	if err != nil || len(page.Events) != 1 || page.NextOffset != int64(len(record)) || page.More {
		t.Fatalf("first page = %+v, err = %v", page, err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString("\"button_down\"}\n")
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	next, err := ReadEvents(path, page.NextOffset)
	if err != nil || len(next.Events) != 1 || string(next.Events[0]) != `{"type":"button_down"}` {
		t.Fatalf("next page = %+v, err = %v", next, err)
	}
}

func TestReadEventsPaginates(t *testing.T) {
	const record = "{\"type\":\"move\"}\n"
	path := writeTrace(t, strings.Repeat(record, EventPageSize+1))
	page, err := ReadEvents(path, 0)
	if err != nil || !page.More || len(page.Events) != EventPageSize || page.NextOffset != int64(EventPageSize*len(record)) {
		t.Fatalf("first page: rows=%d offset=%d more=%v err=%v", len(page.Events), page.NextOffset, page.More, err)
	}
	if _, err := ReadEvents(path, 1); !errors.Is(err, ErrInvalidOffset) {
		t.Fatalf("offset inside a record: err = %v, want ErrInvalidOffset", err)
	}
}
