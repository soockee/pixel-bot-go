package debug

import (
	"encoding/json"
	"net/http"
	"time"
)

// StatePath is the HTTP path serving the live application state snapshot.
const StatePath = "/debug/state"

// StateFunc returns a JSON-serializable snapshot of the running application.
type StateFunc func() map[string]any

// StateHandler returns an http.Handler that serves the snapshot as JSON.
// A "time" field with the current UTC timestamp is added to every response.
func StateHandler(fn StateFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		snap := map[string]any{}
		if fn != nil {
			for k, v := range fn() {
				snap[k] = v
			}
		}
		snap["time"] = time.Now().UTC().Format(time.RFC3339Nano)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snap)
	})
}

// RegisterState exposes the snapshot on the default mux at StatePath, next to
// the pprof handlers. Call at most once per process.
func RegisterState(fn StateFunc) {
	http.Handle(StatePath, StateHandler(fn))
}
