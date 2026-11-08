// Package inputtiming loads and samples frozen human timing profiles for dry runs.
package inputtiming

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
)

const (
	// Version identifies the frozen model format.
	Version = 1
	// FixedHoldUS and FixedDelayUS describe the fixed reel baseline.
	FixedHoldUS  = 30_000
	FixedDelayUS = 300_000
)

// Session identifies the trace and review used in a split.
type Session struct {
	Name         string `json:"name"`
	TraceSHA256  string `json:"trace_sha256"`
	ReviewSHA256 string `json:"review_sha256,omitempty"`
	Samples      int    `json:"eligible_samples"`
}

// Sample is a verified reel hold and its following cooldown, if observed.
// Zero CooldownUS uses the configured cooldown.
type Sample struct {
	HoldUS     int64     `json:"hold_us"`
	CooldownUS int64     `json:"cooldown_us,omitempty"`
	ReactionUS *[2]int64 `json:"reaction_us,omitempty"`
}

// Valid reports whether the sample is within the simulator's timing bounds.
func (s Sample) Valid() bool {
	return s.HoldUS >= 10_000 && s.HoldUS <= 500_000 &&
		(s.CooldownUS == 0 || s.CooldownUS >= 250_000 && s.CooldownUS <= 60_000_000) &&
		(s.ReactionUS == nil || ValidReaction(*s.ReactionUS))
}

// ValidReaction checks the range and precision of a reviewed visual cue.
func ValidReaction(r [2]int64) bool {
	return r[0] >= 0 && r[1] >= r[0] && r[1] <= 2_000_000 && r[1]-r[0] <= 250_000
}

// Calibration measures cue-to-detector lag in an independent simulator trial.
type Calibration struct {
	Detector     string `json:"detector"`
	ReportSHA256 string `json:"report_sha256"`
	LagUS        int64  `json:"lag_us"`
	Samples      int    `json:"samples"`
}

// Valid reports whether the calibration has enough usable observations.
func (c Calibration) Valid() bool {
	return c.Detector != "" && c.ReportSHA256 != "" && c.LagUS >= 0 && c.LagUS <= 1_500_000 && c.Samples >= 5
}

// DelayUS returns the cue-to-press midpoint less calibrated detector lag.
// Zero selects the fixed delay when the cue or calibration is unavailable.
func (c *Calibration) DelayUS(detector string, s Sample) int64 {
	if c == nil || c.Detector != detector || s.ReactionUS == nil {
		return 0
	}
	r := *s.ReactionUS
	return min(1_500_000, max(1, (r[0]+r[1])/2-c.LagUS))
}

// Model is a frozen set of reviewed training observations.
type Model struct {
	Version     int          `json:"version"`
	Training    []Session    `json:"training"`
	Holdout     []Session    `json:"holdout"`
	Samples     []Sample     `json:"samples"`
	Calibration *Calibration `json:"calibration,omitempty"`
}

// Load reads a versioned timing model and checks all samples before use.
func Load(path string) (Model, error) {
	var m Model
	raw, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, err
	}
	if m.Version != Version || len(m.Samples) == 0 {
		return m, errors.New("unsupported or empty timing model")
	}
	if m.Calibration != nil && !m.Calibration.Valid() {
		return m, errors.New("invalid timing calibration")
	}
	for i, s := range m.Samples {
		if !s.Valid() {
			return m, fmt.Errorf("timing sample %d is outside the supported bounds", i)
		}
	}
	return m, nil
}

// Sampler returns a deterministic stream for a single simulator run.
// Its caller serializes calls.
func (m Model) Sampler(seed uint64) func() Sample {
	rng := rand.New(rand.NewPCG(seed, 0))
	return func() Sample { return m.Samples[rng.IntN(len(m.Samples))] }
}
