package debug

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestStateHandlerServesSnapshot(t *testing.T) {
	h := StateHandler(func() map[string]any { return map[string]any{"fsm_state": "monitoring"} })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", StatePath, nil))
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if got["fsm_state"] != "monitoring" {
		t.Fatalf("missing fsm_state: %v", got)
	}
	if _, ok := got["time"]; !ok {
		t.Fatalf("missing time field")
	}
}
