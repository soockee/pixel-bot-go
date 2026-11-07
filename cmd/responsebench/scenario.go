package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math/rand/v2"

	"github.com/soocke/pixel-bot-go/internal/fakegame"
	"github.com/soocke/pixel-bot-go/internal/playtrace"
	"github.com/soocke/pixel-bot-go/internal/traceobserver"
)

//go:embed config.json
var defaultConfig []byte

// Config is the complete experiment definition. It is saved with every run.
type Config struct {
	Bootstrap struct {
		Seed      uint64 `json:"seed"`
		Resamples int    `json:"resamples"`
	} `json:"bootstrap"`
	Splits      map[string]Split `json:"splits"`
	Environment Environment      `json:"environment"`
	Families    []Family         `json:"families"`
	Selection   struct {
		MaxActionsPerEvent float64 `json:"max_actions_per_event"`
	} `json:"selection"`
	Frozen        []string             `json:"frozen"` // baseline first, then the treatment
	Candidates    []ControllerConfig   `json:"candidates"`
	TraceFeedback *traceFeedbackConfig `json:"trace_feedback,omitempty"`
	Shadow        *shadowConfig        `json:"shadow,omitempty"`
	selectedSeeds []uint64
	fittedShadow  *shadowModel
	commonTrace   *playtrace.Writer
}

type Split = traceobserver.Split

// Environment holds the settings shared by every scenario.
type Environment struct {
	Events           int   `json:"events"`
	WindowMs         int64 `json:"window_ms"`          // arrival to deadline
	SplashMs         int   `json:"splash_ms"`          // timeline marker length and minimum arrival spacing
	ServiceMs        int64 `json:"service_ms"`         // action duration when the action gets a reply
	AttemptTimeoutMs int64 `json:"attempt_timeout_ms"` // action duration when it hangs
}

// Family is one workload. Zero fault gaps disable that fault kind, and a zero
// BurstMax gives single events.
type Family struct {
	Name       string `json:"name"`
	MeanGapMs  int    `json:"mean_gap_ms"`
	BurstMin   int    `json:"burst_min,omitempty"`
	BurstMax   int    `json:"burst_max,omitempty"`
	BurstGapMs int    `json:"burst_gap_ms,omitempty"`
	FailGapMs  int    `json:"fail_gap_ms,omitempty"`
	FailMeanMs int    `json:"fail_mean_ms,omitempty"`
	HangGapMs  int    `json:"hang_gap_ms,omitempty"`
	HangMeanMs int    `json:"hang_mean_ms,omitempty"`
}

// Fault makes every action started in [StartMs, EndMs) fail or hang. Faults
// depend only on virtual time, so no controller can change what the other sees.
type Fault struct {
	StartMs int64  `json:"start_ms"`
	EndMs   int64  `json:"end_ms"`
	Kind    string `json:"kind"`
}

// Fault kinds.
const (
	faultFail = "fail"
	faultHang = "hang"
)

// Scenario is one generated workload. Timeline uses the fakegame model: each
// splash segment starts one event. A separate window is needed because the
// response windows of clustered events overlap, which a timeline cannot express.
type Scenario struct {
	ID          string             `json:"id"`
	Family      string             `json:"family"`
	Split       string             `json:"split"`
	Seed        uint64             `json:"seed"`
	Timeline    []fakegame.Segment `json:"timeline"`
	Faults      []Fault            `json:"faults"`
	Environment Environment        `json:"environment"`
}

// parseConfig decodes and validates an experiment definition.
func parseConfig(data []byte) (Config, error) {
	var c Config
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, fmt.Errorf("parse config: %w", err)
	}
	return c, c.validate()
}

func (c Config) validate() error {
	dev, eval := c.Splits["dev"], c.Splits["eval"]
	if dev.Runs <= 0 || eval.Runs <= 0 {
		return fmt.Errorf("config: dev and eval splits need runs")
	}
	if dev.BaseSeed < eval.BaseSeed+uint64(eval.Runs) && eval.BaseSeed < dev.BaseSeed+uint64(dev.Runs) {
		return fmt.Errorf("config: dev and eval seeds overlap")
	}
	e := c.Environment
	if e.Events <= 0 || e.WindowMs <= 0 || e.SplashMs <= 0 || e.ServiceMs <= 0 || e.AttemptTimeoutMs <= 0 {
		return fmt.Errorf("config: environment values must be positive")
	}
	if c.Bootstrap.Resamples <= 0 || len(c.Families) == 0 {
		return fmt.Errorf("config: needs bootstrap resamples and families")
	}
	if c.Selection.MaxActionsPerEvent <= 0 {
		return fmt.Errorf("config: selection needs max_actions_per_event")
	}
	for _, f := range c.Families {
		if f.MeanGapMs <= 0 || f.BurstMax < f.BurstMin ||
			(f.BurstMax > 0 && (f.BurstMin <= 0 || f.BurstGapMs <= 0)) ||
			(f.FailGapMs > 0 && f.FailMeanMs <= 0) || (f.HangGapMs > 0 && f.HangMeanMs <= 0) {
			return fmt.Errorf("config: invalid family %q", f.Name)
		}
	}
	seen := map[string]bool{}
	for _, cc := range c.Candidates {
		if seen[cc.Name] {
			return fmt.Errorf("config: duplicate controller %q", cc.Name)
		}
		seen[cc.Name] = true
		if err := cc.validate(); err != nil {
			return err
		}
	}
	if len(c.Frozen) != 2 || !seen[c.Frozen[0]] || !seen[c.Frozen[1]] {
		return fmt.Errorf("config: frozen must name two candidates")
	}
	if c.TraceFeedback != nil {
		if err := c.TraceFeedback.validate(); err != nil {
			return err
		}
	}
	if c.Shadow != nil {
		return c.Shadow.validate(c)
	}
	return nil
}

// generate builds a scenario from its family, split and seed alone. Controllers
// are not involved, so every controller in a pair gets the same input.
func generate(c Config, fam Family, split string, seed uint64) Scenario {
	rng := rand.New(rand.NewPCG(seed, nameHash(fam.Name)))
	env := c.Environment
	var timeline []fakegame.Segment
	end := int64(0)
	addEvent := func(gapMs int) {
		gapMs = max(gapMs, env.SplashMs)
		if gapMs > env.SplashMs {
			timeline = append(timeline, fakegame.Segment{Phase: fakegame.PhaseBobber, MS: gapMs - env.SplashMs})
		}
		timeline = append(timeline, fakegame.Segment{Phase: fakegame.PhaseSplash, MS: env.SplashMs})
		end += int64(gapMs)
	}
	for n := 0; n < env.Events; {
		addEvent(exponential(rng, fam.MeanGapMs))
		n++
		if fam.BurstMax == 0 {
			continue
		}
		extra := fam.BurstMin - 1 + rng.IntN(fam.BurstMax-fam.BurstMin+1)
		for i := 0; i < extra && n < env.Events; i++ {
			addEvent(exponential(rng, fam.BurstGapMs))
			n++
		}
	}
	end += env.WindowMs + env.AttemptTimeoutMs
	faults := append(faultWindows(rng, faultFail, fam.FailGapMs, fam.FailMeanMs, end),
		faultWindows(rng, faultHang, fam.HangGapMs, fam.HangMeanMs, end)...)
	return Scenario{
		ID:     fmt.Sprintf("%s-%s-%d", split, fam.Name, seed),
		Family: fam.Name, Split: split, Seed: seed,
		Timeline: timeline, Faults: faults, Environment: env,
	}
}

// faultWindows draws fault starts with exponential gaps and durations.
func faultWindows(rng *rand.Rand, kind string, gapMs, meanMs int, end int64) []Fault {
	var faults []Fault
	if gapMs <= 0 {
		return faults
	}
	for t := int64(exponential(rng, gapMs)); t < end; t += int64(exponential(rng, gapMs)) {
		faults = append(faults, Fault{StartMs: t, EndMs: t + 1 + int64(exponential(rng, meanMs)), Kind: kind})
	}
	return faults
}

func exponential(rng *rand.Rand, mean int) int {
	return int(rng.ExpFloat64() * float64(mean))
}

// nameHash gives each name its own random stream without depending on list order.
func nameHash(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

// arrivals returns the start time of every splash segment.
func (s Scenario) arrivals() []int64 {
	var out []int64
	t := int64(0)
	for _, seg := range s.Timeline {
		if seg.Phase == fakegame.PhaseSplash {
			out = append(out, t)
		}
		t += int64(seg.MS)
	}
	return out
}

// faultAt returns the fault active at t. A hang wins over a failure.
func (s Scenario) faultAt(t int64) string {
	active := ""
	for _, f := range s.Faults {
		if t >= f.StartMs && t < f.EndMs {
			if f.Kind == faultHang {
				return faultHang
			}
			active = f.Kind
		}
	}
	return active
}
