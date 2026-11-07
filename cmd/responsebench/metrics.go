package main

import (
	"cmp"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
)

// Metrics is one controller's result on one scenario. Rates use every scenario
// event as the denominator. It is computed only from the trace, so a reviewer
// can recompute it from the saved file.
type Metrics struct {
	ScenarioID string        `json:"scenario_id"`
	Family     string        `json:"family"`
	Seed       uint64        `json:"seed"`
	Controller string        `json:"controller"`
	Events     int           `json:"events"`
	Completed  int           `json:"completed"`
	TimedOut   int           `json:"timed_out"`
	Missed     int           `json:"missed"`
	Actions    int           `json:"actions"`
	Retries    int           `json:"retries"`
	Late       int           `json:"late_replies"`
	SimMs      int64         `json:"sim_ms"`
	TraceScore *traceScore   `json:"trace_score,omitempty"`
	Shadow     *shadowResult `json:"shadow,omitempty"`
	// Latency over completed events only; null when nothing completed.
	CompletedP50Ms *int64 `json:"latency_completed_p50_ms"`
	CompletedP95Ms *int64 `json:"latency_completed_p95_ms"`
	// Latency over all events, with unanswered events censored at the window.
	CensoredP50Ms int64 `json:"latency_censored_p50_ms"`
	CensoredP95Ms int64 `json:"latency_censored_p95_ms"`
}

func (m Metrics) rate(n int) float64 { return float64(n) / float64(m.Events) }

// computeMetrics derives run metrics from a trace and rejects a trace whose
// events do not each have exactly one arrival and one close.
func computeMetrics(sc Scenario, name string, trace []Record) (Metrics, error) {
	m := Metrics{ScenarioID: sc.ID, Family: sc.Family, Seed: sc.Seed, Controller: name, Events: len(sc.arrivals())}
	arrived := make([]int64, m.Events)
	for i := range arrived {
		arrived[i] = -1
	}
	closed := make([]bool, m.Events)
	acted := make([]bool, m.Events)
	var completed []int64
	censored := make([]int64, 0, m.Events)
	for _, r := range trace {
		m.SimMs = max(m.SimMs, r.TMs)
		if r.EventID < 0 || r.EventID >= m.Events {
			return m, fmt.Errorf("%s/%s: unknown event %d", sc.ID, name, r.EventID)
		}
		switch r.Action {
		case actionArrive:
			arrived[r.EventID] = r.TMs
		case actionAttempt:
			if arrived[r.EventID] < 0 || closed[r.EventID] {
				return m, fmt.Errorf("%s/%s: action outside event %d", sc.ID, name, r.EventID)
			}
			m.Actions++
			if acted[r.EventID] {
				m.Retries++
			}
			acted[r.EventID] = true
		case actionReply:
			if r.Outcome == outcomeLate {
				m.Late++
			}
		case actionClose:
			if arrived[r.EventID] < 0 || closed[r.EventID] {
				return m, fmt.Errorf("%s/%s: bad close for event %d", sc.ID, name, r.EventID)
			}
			closed[r.EventID] = true
			latency := sc.Environment.WindowMs
			switch r.Outcome {
			case outcomeCompleted:
				m.Completed++
				latency = r.TMs - arrived[r.EventID]
				completed = append(completed, latency)
			case outcomeTimedOut:
				m.TimedOut++
			case outcomeMissed:
				m.Missed++
			default:
				return m, fmt.Errorf("%s/%s: unknown close %q", sc.ID, name, r.Outcome)
			}
			censored = append(censored, latency)
		}
	}
	if len(censored) != m.Events {
		return m, fmt.Errorf("%s/%s: %d of %d events closed", sc.ID, name, len(censored), m.Events)
	}
	if len(completed) > 0 {
		p50, p95 := nearestRank(completed, 50), nearestRank(completed, 95)
		m.CompletedP50Ms, m.CompletedP95Ms = &p50, &p95
	}
	m.CensoredP50Ms, m.CensoredP95Ms = nearestRank(censored, 50), nearestRank(censored, 95)
	return m, nil
}

// nearestRank returns the smallest value with at least p percent of values at or
// below it. Results are always observed values. It sorts values in place.
func nearestRank(values []int64, p int) int64 {
	slices.Sort(values)
	return values[(len(values)*p+99)/100-1]
}

// metricDefs lists the reported metrics. The bool is false when a run has no
// value, which only happens for latency over completed events.
var metricDefs = []struct {
	name string
	get  func(Metrics) (float64, bool)
}{
	{"completion_rate", func(m Metrics) (float64, bool) { return m.rate(m.Completed), true }},
	{"timeout_rate", func(m Metrics) (float64, bool) { return m.rate(m.TimedOut), true }},
	{"missed_rate", func(m Metrics) (float64, bool) { return m.rate(m.Missed), true }},
	{"zero_completion_run", func(m Metrics) (float64, bool) {
		if m.Completed == 0 {
			return 1, true
		}
		return 0, true
	}},
	{"retries_per_event", func(m Metrics) (float64, bool) { return m.rate(m.Retries), true }},
	{"actions_per_event", func(m Metrics) (float64, bool) { return m.rate(m.Actions), true }},
	{"actions_per_sim_minute", func(m Metrics) (float64, bool) {
		return float64(m.Actions) * 60000 / float64(m.SimMs), true
	}},
	{"latency_censored_p50_ms", func(m Metrics) (float64, bool) { return float64(m.CensoredP50Ms), true }},
	{"latency_censored_p95_ms", func(m Metrics) (float64, bool) { return float64(m.CensoredP95Ms), true }},
	{"latency_completed_p50_ms", func(m Metrics) (float64, bool) { return optional(m.CompletedP50Ms) }},
	{"latency_completed_p95_ms", func(m Metrics) (float64, bool) { return optional(m.CompletedP95Ms) }},
}

func optional(v *int64) (float64, bool) {
	if v == nil {
		return 0, false
	}
	return float64(*v), true
}

// Comparison is the paired result for one family and metric. Differences are
// treatment minus baseline, computed per scenario.
type Comparison struct {
	Family     string  `json:"family"`
	Metric     string  `json:"metric"`
	Pairs      int     `json:"pairs"`
	Excluded   int     `json:"excluded_pairs"` // a side had no value
	Baseline   float64 `json:"baseline_mean"`
	Treatment  float64 `json:"treatment_mean"`
	MeanDiff   float64 `json:"mean_diff"`
	CI95Lo     float64 `json:"ci95_lo"`
	CI95Hi     float64 `json:"ci95_hi"`
	HigherRuns int     `json:"treatment_higher_runs"`
	LowerRuns  int     `json:"treatment_lower_runs"`
	EqualRuns  int     `json:"equal_runs"`
}

// compare pairs runs by scenario for each family, plus an "all" row, and fails
// if any scenario lacks a result from either controller.
func compare(cfg Config, metrics []Metrics, baseline, treatment string) ([]Comparison, error) {
	base := map[string]Metrics{}
	treat := map[string]Metrics{}
	var ids []string
	for _, m := range metrics {
		switch m.Controller {
		case baseline:
			base[m.ScenarioID] = m
			ids = append(ids, m.ScenarioID)
		case treatment:
			treat[m.ScenarioID] = m
		}
	}
	if len(base) == 0 || len(base) != len(treat) {
		return nil, fmt.Errorf("unpaired runs: %d baseline, %d treatment", len(base), len(treat))
	}

	families := []string{"all"}
	for _, f := range cfg.Families {
		families = append(families, f.Name)
	}
	defs := metricDefs
	if metrics[0].TraceScore != nil {
		defs = append(slices.Clone(defs), traceMetricDefs...)
	}
	if metrics[0].Shadow != nil {
		defs = append(defs, shadowMetricDefs()...)
	}
	var out []Comparison
	for _, family := range families {
		for _, def := range defs {
			c := Comparison{Family: family, Metric: def.name}
			var diffs []float64
			for _, id := range ids {
				a, b := base[id], treat[id]
				if family != "all" && a.Family != family {
					continue
				}
				if _, ok := treat[id]; !ok {
					return nil, fmt.Errorf("scenario %s has no %s run", id, treatment)
				}
				va, okA := def.get(a)
				vb, okB := def.get(b)
				if !okA || !okB {
					c.Excluded++
					continue
				}
				c.Baseline += va
				c.Treatment += vb
				d := vb - va
				diffs = append(diffs, d)
				switch {
				case d > 0:
					c.HigherRuns++
				case d < 0:
					c.LowerRuns++
				default:
					c.EqualRuns++
				}
			}
			c.Pairs = len(diffs)
			if c.Pairs > 0 {
				c.Baseline /= float64(c.Pairs)
				c.Treatment /= float64(c.Pairs)
				c.MeanDiff = mean(diffs)
				seed := cfg.Bootstrap.Seed ^ nameHash(family+"/"+def.name)
				c.CI95Lo, c.CI95Hi = bootstrapCI(diffs, seed, cfg.Bootstrap.Resamples)
			}
			out = append(out, c)
		}
	}
	return out, nil
}

func mean(values []float64) float64 {
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// bootstrapCI returns the 95% percentile interval of the mean, resampling pairs.
func bootstrapCI(diffs []float64, seed uint64, resamples int) (lo, hi float64) {
	rng := rand.New(rand.NewPCG(seed, 0))
	means := make([]float64, resamples)
	for i := range means {
		sum := 0.0
		for range diffs {
			sum += diffs[rng.IntN(len(diffs))]
		}
		means[i] = sum / float64(len(diffs))
	}
	slices.Sort(means)
	return means[resamples*25/1000], means[min(resamples*975/1000, resamples-1)]
}

// Score ranks one candidate on the development split.
type Score struct {
	Name            string  `json:"name"`
	Kind            string  `json:"kind"`
	Completion      float64 `json:"mean_completion_rate"`
	CensoredP95Ms   float64 `json:"mean_latency_censored_p95_ms"`
	ActionsPerEvent float64 `json:"mean_actions_per_event"`
	WithinBudget    bool    `json:"within_budget"`
}

// Selection is the development outcome: every score, best first, and the winners.
type Selection struct {
	Scores []Score  `json:"scores"`
	Frozen []string `json:"selected"` // fixed winner, then backoff winner
}

// selectControllers applies the rule fixed before development runs: stay within
// the action budget, then take the highest completion at 0.1% resolution, then
// the lowest censored p95 latency, then the fewest actions, then the name.
func selectControllers(cfg Config, metrics []Metrics) (Selection, error) {
	var sel Selection
	for _, cc := range cfg.Candidates {
		var completion, p95, actions []float64
		for _, m := range metrics {
			if m.Controller != cc.Name {
				continue
			}
			completion = append(completion, m.rate(m.Completed))
			p95 = append(p95, float64(m.CensoredP95Ms))
			actions = append(actions, m.rate(m.Actions))
		}
		if len(completion) == 0 {
			return sel, fmt.Errorf("no runs for candidate %s", cc.Name)
		}
		s := Score{Name: cc.Name, Kind: cc.Kind, Completion: mean(completion), CensoredP95Ms: mean(p95), ActionsPerEvent: mean(actions)}
		s.WithinBudget = s.ActionsPerEvent <= cfg.Selection.MaxActionsPerEvent
		sel.Scores = append(sel.Scores, s)
	}
	slices.SortFunc(sel.Scores, func(a, b Score) int {
		order := func(ok bool) int {
			if ok {
				return 0
			}
			return 1
		}
		return cmp.Or(
			cmp.Compare(order(a.WithinBudget), order(b.WithinBudget)),
			cmp.Compare(math.Round(b.Completion*1000), math.Round(a.Completion*1000)),
			cmp.Compare(a.CensoredP95Ms, b.CensoredP95Ms),
			cmp.Compare(a.ActionsPerEvent, b.ActionsPerEvent),
			cmp.Compare(a.Name, b.Name),
		)
	})
	for _, kind := range []string{kindFixed, kindBackoff} {
		i := slices.IndexFunc(sel.Scores, func(s Score) bool { return s.Kind == kind && s.WithinBudget })
		if i < 0 {
			return sel, fmt.Errorf("no %s candidate within the action budget", kind)
		}
		sel.Frozen = append(sel.Frozen, sel.Scores[i].Name)
	}
	return sel, nil
}

const definitions = `## Definitions

- Every event closes exactly once: completed, timed_out or missed. The three rates use all scenario events as the denominator, so failed runs stay in every rate.
- completed: a successful reply arrived at or before the deadline (arrival + window_ms).
- timed_out: at least one action started, but no success arrived by the deadline. This includes events a controller gave up on and successful replies after the deadline.
- missed: no action started before the deadline.
- Response latency: arrival to successful reply.
- latency_completed: completed events only. This excludes the hardest events, so read it with the completion rate.
- latency_censored: all events, with each unanswered event set to window_ms. It is a lower bound on the true response time.
- Percentiles use the nearest-rank method on each run. Tables show the mean of the per-run values.
- retries: actions beyond the first on an event.
- Differences are treatment minus baseline, per scenario. Intervals are 95% percentile paired bootstrap intervals, not adjusted for multiple comparisons.
- excluded_pairs: pairs where a side had no latency_completed value. A zero-completion run is counted in zero_completion_run and in the rates.
`

// comparisonReport renders the paired results as Markdown.
func comparisonReport(split, baseline, treatment string, rows []Comparison) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Response benchmark: %s split\n\nBaseline: `%s`. Treatment: `%s`.\n\n%s", split, baseline, treatment, definitions)
	family := ""
	for _, r := range rows {
		if r.Family != family {
			family = r.Family
			fmt.Fprintf(&b, "\n## %s\n\n| Metric | Pairs | Excluded | Baseline | Treatment | Mean diff | 95%% CI | Higher / lower / equal runs |\n|---|---|---|---|---|---|---|---|\n", family)
		}
		fmt.Fprintf(&b, "| %s | %d | %d | %.4g | %.4g | %.4g | [%.4g, %.4g] | %d / %d / %d |\n",
			r.Metric, r.Pairs, r.Excluded, r.Baseline, r.Treatment, r.MeanDiff, r.CI95Lo, r.CI95Hi, r.HigherRuns, r.LowerRuns, r.EqualRuns)
	}
	return b.String()
}

// selectionReport renders the development ranking as Markdown.
func selectionReport(sel Selection) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Response benchmark: dev split\n\nSelected: `%s`, `%s`.\n\n| Candidate | Within budget | Completion | Censored p95 ms | Actions per event |\n|---|---|---|---|---|\n", sel.Frozen[0], sel.Frozen[1])
	for _, s := range sel.Scores {
		fmt.Fprintf(&b, "| %s | %t | %.4f | %.1f | %.3f |\n", s.Name, s.WithinBudget, s.Completion, s.CensoredP95Ms, s.ActionsPerEvent)
	}
	return b.String()
}
