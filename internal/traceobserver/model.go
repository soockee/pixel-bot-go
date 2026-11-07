package traceobserver

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
)

// Split is a seed range. Dev and eval ranges must not overlap.
type Split struct {
	BaseSeed uint64 `json:"base_seed"`
	Runs     int    `json:"runs"`
}

// Config identifies the reference data and fixed clock used by a frozen model.
type Config struct {
	Train           Split    `json:"train"`
	Calibration     Split    `json:"calibration"`
	HoldoutFamilies []string `json:"holdout_families"`
	FalseAlarmRate  float64  `json:"false_alarm_rate"`
	TickMs          int64    `json:"tick_ms"`
	ScoreAgeMs      int64    `json:"score_age_ms"`
}

const (
	timingBinMs    = 25
	timingBins     = 40
	timingContexts = 6
	timingWindow   = 12
)

type hazardBin struct {
	Exposures int `json:"exposures"`
	Actions   int `json:"actions"`
}

type timingModel [timingContexts][timingBins]hazardBin

type timingSample struct {
	context int
	delay   int64
	action  bool
}

// Learn adds one observed or censored wait to the timing hazard counts.
func (m *timingModel) Learn(s timingSample) {
	row := &m[s.context]
	bin := min(int(s.delay/timingBinMs), len(row)-1)
	for i := 0; i < bin; i++ {
		row[i].Exposures++
	}
	if s.action {
		row[bin].Exposures++
		row[bin].Actions++
	}
}

func (m *timingModel) surprise(s timingSample) float64 {
	row := &m[s.context]
	bin := min(int(s.delay/timingBinMs), len(row)-1)
	var score float64
	for i := 0; i < bin; i++ {
		score -= math.Log1p(-row[i].probability())
	}
	if s.action {
		score -= math.Log(row[bin].probability())
	}
	return score
}

func (b hazardBin) probability() float64 {
	return float64(b.Actions+1) / float64(b.Exposures+2)
}

type pendingWait struct {
	start   int64
	context int
}

// TimingStream turns raw records into completed or censored waits.
type TimingStream struct {
	waits    map[int]pendingWait
	lastBite int64
	sawBite  bool
	dense    bool
}

// Observe consumes a trace record and returns a wait when one ends.
func (s *TimingStream) Observe(r Record) (timingSample, bool) {
	switch r.Action {
	case ActionArrive:
		s.dense = s.sawBite && r.TMs-s.lastBite <= 250
		s.lastBite, s.sawBite = r.TMs, true
		context := 0
		if s.dense {
			context++
		}
		s.begin(r.EventID, pendingWait{r.TMs, context})
	case ActionReply:
		if r.Outcome == OutcomeFail || r.Outcome == OutcomeTimeout {
			context := 2
			if r.Outcome == OutcomeTimeout {
				context = 4
			}
			if s.dense && r.TMs-s.lastBite <= 250 {
				context++
			}
			s.begin(r.EventID, pendingWait{r.TMs, context})
		}
	case ActionAttempt, ActionClose:
		wait, ok := s.waits[r.EventID]
		delete(s.waits, r.EventID)
		if ok {
			return timingSample{wait.context, r.TMs - wait.start, r.Action == ActionAttempt}, true
		}
	}
	return timingSample{}, false
}

// Threshold stores a cutoff and its synthetic calibration counts.
type Threshold struct {
	Cutoff          float64 `json:"cutoff"`
	ReferenceRuns   int     `json:"reference_runs"`
	ReferenceAlerts int     `json:"reference_alerts"`
}

// Model contains the frozen detector settings and timing hazard counts.
type Model struct {
	Version           int         `json:"version"`
	Config            Config      `json:"config"`
	Timing            timingModel `json:"timing_hazards"`
	TimingThreshold   Threshold   `json:"timing_threshold"`
	SequenceThreshold Threshold   `json:"sequence_threshold"`
}

func (s *TimingStream) begin(id int, wait pendingWait) {
	if s.waits == nil {
		s.waits = make(map[int]pendingWait)
	}
	s.waits[id] = wait
}

// LoadModel loads the frozen artifact emitted by responsebench.
func LoadModel(path string) (*Model, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var model Model
	if err := json.Unmarshal(data, &model); err != nil {
		return nil, err
	}
	if model.Version != 1 || model.Config.TickMs <= 0 || model.Config.ScoreAgeMs < model.Config.TickMs ||
		model.TimingThreshold.ReferenceRuns <= 0 || model.SequenceThreshold.ReferenceRuns <= 0 ||
		model.TimingThreshold.Cutoff < 0 || model.SequenceThreshold.Cutoff < 0 || model.SequenceThreshold.Cutoff > 1 {
		return nil, fmt.Errorf("invalid trace observer model")
	}
	for _, row := range model.Timing {
		for _, bin := range row {
			if bin.Actions < 0 || bin.Exposures < bin.Actions {
				return nil, fmt.Errorf("invalid trace observer hazard counts")
			}
		}
	}
	return &model, nil
}
