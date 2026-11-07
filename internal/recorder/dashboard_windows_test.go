package recorder

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createDashboardTrace(t *testing.T, content string) *os.File {
	t.Helper()
	file, err := os.Create(filepath.Join(t.TempDir(), "test.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	if _, err := file.WriteString(content); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestDashboardReadsOnlyCompleteRecords(t *testing.T) {
	const record = "{\"type\":\"move\"}\n"
	file := createDashboardTrace(t, record+"{\"type\":")
	page, err := readEventPage(file, 0, true)
	if err != nil || len(page.Events) != 1 || page.NextOffset != int64(len(record)) || !page.Live || page.More {
		t.Fatalf("first page = %+v, err = %v", page, err)
	}
	if _, err := file.WriteString("\"button_down\"}\n"); err != nil {
		t.Fatal(err)
	}
	next, err := readEventPage(file, page.NextOffset, true)
	if err != nil || len(next.Events) != 1 || string(next.Events[0]) != `{"type":"button_down"}` {
		t.Fatalf("next page = %+v, err = %v", next, err)
	}
}

func TestDashboardPaginatesAndRequiresLocalHost(t *testing.T) {
	const address = "127.0.0.1:6070"
	const record = "{\"type\":\"move\"}\n"
	file := createDashboardTrace(t, strings.Repeat(record, eventPageSize+1))
	handler := dashboardHandler(file, "", false, address)
	call := func(host, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "http://"+host+path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	w := call(address, "/api/events")
	var page eventPage
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || page.Live || !page.More ||
		len(page.Events) != eventPageSize || page.NextOffset != int64(eventPageSize*len(record)) {
		t.Fatalf("unexpected first page: status=%d rows=%d offset=%d", w.Code, len(page.Events), page.NextOffset)
	}
	for _, check := range []struct {
		host, path string
		code       int
	}{
		{address, "/", http.StatusOK},
		{address, "/api/events?offset=1", http.StatusBadRequest},
		{"external.example", "/api/events", http.StatusForbidden},
		{address, "/test.jsonl", http.StatusNotFound},
	} {
		if got := call(check.host, check.path).Code; got != check.code {
			t.Errorf("%s %s: got %d, want %d", check.host, check.path, got, check.code)
		}
	}
}

func TestDashboardRejectsNonLoopbackAddress(t *testing.T) {
	listener, err := listenDashboard("0.0.0.0:0")
	if err == nil {
		_ = listener.Close()
		t.Fatal("dashboard accepted a non-loopback address")
	}
}
