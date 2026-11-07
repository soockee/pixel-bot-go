package traceobserver

// Trace actions.
const (
	ActionArrive  = "arrive"
	ActionAttempt = "attempt"
	ActionReply   = "reply"
	ActionClose   = "close"
)

// Trace outcomes. A reply is ok, fail, timeout or late; a close is completed,
// timed_out or missed. Dry-run dispatches have no server outcome.
const (
	OutcomeOK               = "ok"
	OutcomeFail             = "fail"
	OutcomeTimeout          = "timeout"
	OutcomeLate             = "late"
	OutcomeCompleted        = "completed"
	OutcomeTimedOut         = "timed_out"
	OutcomeMissed           = "missed"
	OutcomeDryRunDispatched = "dry_run_dispatched"
)

// Record is one raw trace line.
type Record struct {
	ScenarioID string `json:"scenario_id"`
	Seed       uint64 `json:"seed"`
	Controller string `json:"controller"`
	TMs        int64  `json:"t_ms"`
	EventID    int    `json:"event_id"`
	Action     string `json:"action"`
	Attempt    int    `json:"attempt,omitempty"`
	Outcome    string `json:"outcome,omitempty"`
}
