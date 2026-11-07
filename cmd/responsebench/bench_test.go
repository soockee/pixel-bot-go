package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/internal/fakegame"
)

func testConfig(t *testing.T) Config {
	t.Helper()
	cfg, err := parseConfig(defaultConfig)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// manualScenario places events at the given times with short, round timing.
func manualScenario(arrivals []int, faults ...Fault) Scenario {
	sc := Scenario{ID: "test", Faults: faults, Environment: Environment{WindowMs: 100, ServiceMs: 10, AttemptTimeoutMs: 40}}
	last := 0
	for _, at := range arrivals {
		if at > last {
			sc.Timeline = append(sc.Timeline, fakegame.Segment{Phase: fakegame.PhaseBobber, MS: at - last})
		}
		sc.Timeline = append(sc.Timeline, fakegame.Segment{Phase: fakegame.PhaseSplash, MS: 1})
		last = at + 1
	}
	return sc
}

// script acts at scripted times, so tests control exact timing.
type script struct {
	acts []int64
	live map[int]bool
}

func (s *script) OnArrive(_ int64, id int) {
	if s.live == nil {
		s.live = map[int]bool{}
	}
	s.live[id] = true
}
func (s *script) OnResult(_ int64, id int, ok bool) {
	if ok {
		delete(s.live, id)
	}
}
func (s *script) OnClosed(id int) { delete(s.live, id) }
func (s *script) Next(now int64) (int, int64, bool) {
	for len(s.acts) > 0 && s.acts[0] < now {
		s.acts = s.acts[1:]
	}
	if len(s.acts) == 0 || len(s.live) == 0 {
		return 0, 0, false
	}
	at := s.acts[0]
	for id := range s.live {
		return id, at, true
	}
	return 0, 0, false
}

func closeOutcome(t *testing.T, trace []Record, id int) Record {
	t.Helper()
	for _, r := range trace {
		if r.Action == actionClose && r.EventID == id {
			return r
		}
	}
	t.Fatalf("event %d never closed", id)
	return Record{}
}

func TestTimeoutAccounting(t *testing.T) {
	cases := []struct {
		name    string
		acts    []int64
		faults  []Fault
		outcome string
		closeAt int64
	}{
		{"success", []int64{5}, nil, outcomeCompleted, 15},
		{"success exactly at deadline", []int64{90}, nil, outcomeCompleted, 100},
		{"success one ms after deadline", []int64{91}, nil, outcomeTimedOut, 100},
		{"all attempts fail", []int64{5, 30}, []Fault{{0, 1000, faultFail}}, outcomeTimedOut, 100},
		{"hang outlives deadline", []int64{70}, []Fault{{0, 1000, faultHang}}, outcomeTimedOut, 100},
		{"no action", nil, nil, outcomeMissed, 100},
		{"action only after deadline", []int64{150}, nil, outcomeMissed, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sc := manualScenario([]int{0}, tc.faults...)
			trace, err := simulate(sc, &script{acts: tc.acts}, "s")
			if err != nil {
				t.Fatal(err)
			}
			if c := closeOutcome(t, trace, 0); c.Outcome != tc.outcome || c.TMs != tc.closeAt {
				t.Fatalf("close = %+v, want %s at %d", c, tc.outcome, tc.closeAt)
			}
			m, err := computeMetrics(sc, "s", trace)
			if err != nil {
				t.Fatal(err)
			}
			if m.Completed+m.TimedOut+m.Missed != m.Events {
				t.Fatalf("outcomes do not sum to events: %+v", m)
			}
		})
	}
}

func TestUnansweredEventsStayInDenominatorsAndLatency(t *testing.T) {
	sc := manualScenario([]int{0, 300, 600})
	trace, err := simulate(sc, &script{acts: []int64{5}}, "s")
	if err != nil {
		t.Fatal(err)
	}
	m, err := computeMetrics(sc, "s", trace)
	if err != nil {
		t.Fatal(err)
	}
	if m.Events != 3 || m.Completed != 1 || m.Missed != 2 || m.rate(m.Completed) != 1.0/3 {
		t.Fatalf("metrics = %+v", m)
	}
	if *m.CompletedP95Ms != 15 || m.CensoredP50Ms != 100 || m.CensoredP95Ms != 100 {
		t.Fatalf("latency: completed p95 %d, censored %d / %d", *m.CompletedP95Ms, m.CensoredP50Ms, m.CensoredP95Ms)
	}

	none, err := computeMetrics(sc, "s", mustSim(t, sc, &script{}))
	if err != nil {
		t.Fatal(err)
	}
	if none.CompletedP50Ms != nil || none.CensoredP95Ms != 100 || none.Missed != 3 {
		t.Fatalf("zero-completion run = %+v", none)
	}
}

func mustSim(t *testing.T, sc Scenario, c Controller) []Record {
	t.Helper()
	trace, err := simulate(sc, c, "s")
	if err != nil {
		t.Fatal(err)
	}
	return trace
}

func TestMetricsRejectBrokenTraces(t *testing.T) {
	sc := manualScenario([]int{0})
	trace := mustSim(t, sc, &script{acts: []int64{5}})
	for name, broken := range map[string][]Record{
		"lost close":   trace[:len(trace)-1],
		"double close": append(slices.Clone(trace), trace[len(trace)-1]),
		"lost arrival": trace[1:],
		"unknown id":   append(slices.Clone(trace), Record{EventID: 9}),
	} {
		if _, err := computeMetrics(sc, "s", broken); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestVirtualClockAndCorrelationOnEveryWorkload(t *testing.T) {
	cfg := testConfig(t)
	for _, fam := range cfg.Families {
		sc := generate(cfg, fam, "dev", 7)
		arrivals := sc.arrivals()
		for _, cc := range cfg.Candidates {
			trace := mustSim(t, sc, newController(cc, sc.Seed))
			openedAt := map[int]int64{}
			closedAt := map[int]int64{}
			var prev int64
			var busyUntil int64
			for _, r := range trace {
				if r.TMs < prev {
					t.Fatalf("%s/%s: time went back at %+v", sc.ID, cc.Name, r)
				}
				prev = r.TMs
				switch r.Action {
				case actionArrive:
					if arrivals[r.EventID] != r.TMs {
						t.Fatalf("%s: arrival %+v, want %d", sc.ID, r, arrivals[r.EventID])
					}
					openedAt[r.EventID] = r.TMs
				case actionAttempt:
					opened, ok := openedAt[r.EventID]
					_, closed := closedAt[r.EventID]
					if !ok || closed || r.TMs < opened || r.TMs < busyUntil {
						t.Fatalf("%s/%s: uncorrelated action %+v", sc.ID, cc.Name, r)
					}
					busyUntil = r.TMs + sc.Environment.ServiceMs
				case actionClose:
					closedAt[r.EventID] = r.TMs
					if r.TMs-openedAt[r.EventID] > sc.Environment.WindowMs {
						t.Fatalf("%s/%s: closed late %+v", sc.ID, cc.Name, r)
					}
				}
			}
			if len(closedAt) != cfg.Environment.Events {
				t.Fatalf("%s/%s: closed %d events", sc.ID, cc.Name, len(closedAt))
			}
		}
	}
}

func TestSameTimeOrderIsFixed(t *testing.T) {
	sim := &simulation{}
	sim.push(step{at: 5, kind: stepWake})
	sim.push(step{at: 5, kind: stepDeadline, id: 2})
	sim.push(step{at: 5, kind: stepDeadline, id: 1})
	sim.push(step{at: 5, kind: stepReply})
	sim.push(step{at: 4, kind: stepArrive})
	want := []struct{ at, kind, id int }{{4, stepArrive, 0}, {5, stepReply, 0}, {5, stepDeadline, 2}, {5, stepDeadline, 1}, {5, stepWake, 0}}
	for i, w := range want {
		s := sim.queue[i]
		if int(s.at) != w.at || s.kind != w.kind || s.id != w.id {
			t.Fatalf("position %d = %+v, want %+v", i, s, w)
		}
	}
}

func TestFixedActsOnlyOnTicks(t *testing.T) {
	sc := manualScenario([]int{3})
	cc := ControllerConfig{Name: "f", Kind: kindFixed, IntervalMs: 20}
	for _, r := range mustSim(t, sc, newController(cc, 1)) {
		if r.Action == actionAttempt && r.TMs%20 != 0 {
			t.Fatalf("off-tick action %+v", r)
		}
	}
}

func TestFixedRotatesFailedEvents(t *testing.T) {
	c := &fixedInterval{interval: 10}
	c.OnArrive(0, 1)
	c.OnArrive(0, 2)
	c.OnResult(5, 1, false)
	if id, _, _ := c.Next(5); id != 2 {
		t.Fatalf("next = %d, want 2", id)
	}
}

func TestBackoffDelaysAreBounded(t *testing.T) {
	cc := ControllerConfig{Name: "b", Kind: kindBackoff, MinDelayMs: 2, MaxDelayMs: 5, BaseBackoffMs: 10, MaxBackoffMs: 25, MaxAttempts: 5}
	c := newController(cc, 1).(*boundedBackoff)
	c.OnArrive(0, 1)
	if _, at, _ := c.Next(0); at < 2 || at > 5 {
		t.Fatalf("first action at %d", at)
	}
	// Backoff doubles from 10, then the 25 cap holds; the pacing gap is at most 5.
	for attempt, want := range []int64{10, 20, 25, 25} {
		c.OnResult(1000, 1, false)
		_, at, ok := c.Next(1000)
		if !ok || c.pending[0].ready != 1000+want || at < 1002 || at > 1000+want+5 {
			t.Fatalf("attempt %d: at=%d ready=%d", attempt, at, c.pending[0].ready)
		}
	}
	c.OnResult(1000, 1, false)
	if _, _, ok := c.Next(1000); ok {
		t.Fatal("controller should give up after MaxAttempts")
	}
}

func TestPercentileUsesNearestRank(t *testing.T) {
	values := []int64{5, 1, 4, 2, 3}
	if got := nearestRank(values, 50); got != 3 {
		t.Fatalf("p50 = %d", got)
	}
	if got := nearestRank(values, 95); got != 5 {
		t.Fatalf("p95 = %d", got)
	}
	if got := nearestRank([]int64{7}, 95); got != 7 {
		t.Fatalf("single value = %d", got)
	}
	hundred := make([]int64, 100)
	for i := range hundred {
		hundred[i] = int64(i + 1)
	}
	if got := nearestRank(hundred, 95); got != 95 {
		t.Fatalf("p95 of 1..100 = %d", got)
	}
}

func TestBootstrapCI(t *testing.T) {
	if lo, hi := bootstrapCI([]float64{2, 2, 2}, 1, 200); lo != 2 || hi != 2 {
		t.Fatalf("constant diffs: [%v, %v]", lo, hi)
	}
	diffs := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	lo, hi := bootstrapCI(diffs, 1, 2000)
	if lo >= mean(diffs) || hi <= mean(diffs) {
		t.Fatalf("interval [%v, %v] should contain the mean", lo, hi)
	}
}

func TestCompareExcludesPairsWithoutLatency(t *testing.T) {
	cfg := testConfig(t)
	cfg.Bootstrap.Resamples = 10
	v := int64(40)
	metrics := []Metrics{
		{ScenarioID: "a", Family: "sparse", Controller: "x", Events: 2, Completed: 1, CompletedP50Ms: &v},
		{ScenarioID: "a", Family: "sparse", Controller: "y", Events: 2},
	}
	rows, err := compare(cfg, metrics, "x", "y")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Family != "sparse" {
			continue
		}
		if r.Metric == "completion_rate" && (r.Pairs != 1 || r.MeanDiff != -0.5) {
			t.Fatalf("completion = %+v", r)
		}
		if r.Metric == "latency_completed_p50_ms" && (r.Pairs != 0 || r.Excluded != 1) {
			t.Fatalf("latency = %+v", r)
		}
	}
	if _, err := compare(cfg, metrics[:1], "x", "y"); err == nil {
		t.Fatal("unpaired runs must be an error")
	}
}

func TestScenariosIgnoreControllersAndSplitsAreDisjoint(t *testing.T) {
	cfg := testConfig(t)
	seen := map[uint64]string{}
	for split, s := range cfg.Splits {
		for i := range s.Runs {
			seed := s.BaseSeed + uint64(i)
			if other, dup := seen[seed]; dup {
				t.Fatalf("seed %d in %s and %s", seed, split, other)
			}
			seen[seed] = split
		}
	}
	bad := Config{Splits: map[string]Split{"dev": {BaseSeed: 1, Runs: 10}, "eval": {BaseSeed: 5, Runs: 10}}}
	if err := bad.validate(); err == nil || !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("overlapping splits: %v", err)
	}

	fam := cfg.Families[len(cfg.Families)-1]
	want := generate(cfg, fam, "eval", 42)
	for _, cc := range cfg.Candidates {
		mustSim(t, want, newController(cc, want.Seed))
	}
	if got := generate(cfg, fam, "eval", 42); !reflect.DeepEqual(got, want) {
		t.Fatal("scenario changed between generations")
	}
	if other := generate(cfg, fam, "eval", 43); reflect.DeepEqual(other.Timeline, want.Timeline) {
		t.Fatal("seed has no effect")
	}
}

func TestTimelineMatchesFakegameModel(t *testing.T) {
	cfg := testConfig(t)
	sc := generate(cfg, cfg.Families[1], "dev", 1)
	if got := len(sc.arrivals()); got != cfg.Environment.Events {
		t.Fatalf("arrivals = %d", got)
	}
	timeline := fakegame.Scenario{Timeline: sc.Timeline}
	for _, at := range sc.arrivals() {
		if phase, _ := timeline.PhaseAt(time.Duration(at) * time.Millisecond); phase != fakegame.PhaseSplash {
			t.Fatalf("phase at %d = %s", at, phase)
		}
	}
}

func TestFaultFamiliesActuallyFault(t *testing.T) {
	cfg := testConfig(t)
	totals := map[string]int{}
	for _, fam := range cfg.Families {
		sc := generate(cfg, fam, "dev", 1)
		for _, f := range sc.Faults {
			totals[fam.Name+"/"+f.Kind]++
		}
	}
	for _, key := range []string{"transient/fail", "timeouts/hang", "mixed/fail", "mixed/hang"} {
		if totals[key] == 0 {
			t.Errorf("no %s faults", key)
		}
	}
	if totals["sparse/fail"]+totals["clustered/hang"] != 0 {
		t.Errorf("fault-free families have faults: %v", totals)
	}
}

// The frozen pair must be what the published rule picks on the dev split.
func TestFrozenSettingsMatchDevSelection(t *testing.T) {
	cfg := testConfig(t)
	metrics, err := runAll(cfg, "dev", cfg.Candidates, &bytes.Buffer{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sel, err := selectControllers(cfg, metrics)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sel.Frozen, cfg.Frozen) {
		t.Fatalf("dev selection %v, frozen %v", sel.Frozen, cfg.Frozen)
	}
}

func TestRunIsByteReproducible(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(t)
	eval := cfg.Splits["eval"]
	eval.Runs = 3
	cfg.Splits["eval"] = eval
	cfg.Bootstrap.Resamples = 100
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	first, second := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	for _, out := range []string{first, second} {
		if err := run(out, "eval", path, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	a, err := os.ReadFile(filepath.Join(first, "CHECKSUMS.txt"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(second, "CHECKSUMS.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) || len(a) == 0 {
		t.Fatalf("runs differ:\n%s\n%s", a, b)
	}
	for _, name := range []string{"config.json", "scenarios.jsonl", "traces.jsonl", "metrics.jsonl", "summary.json", "summary.md", "manifest.json"} {
		if info, err := os.Stat(filepath.Join(first, name)); err != nil || info.Size() == 0 {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := run(first, "eval", path, &bytes.Buffer{}); err == nil {
		t.Fatal("an existing result directory must not be overwritten")
	}
}

func TestSavedTraceRecomputesSavedMetrics(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(t)
	eval := cfg.Splits["eval"]
	eval.Runs = 2
	cfg.Splits["eval"] = eval
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if err := run(out, "eval", path, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	scenarios := map[string]Scenario{}
	for _, sc := range readJSONL[Scenario](t, filepath.Join(out, "scenarios.jsonl")) {
		scenarios[sc.ID] = sc
	}
	traces := map[[2]string][]Record{}
	for _, r := range readJSONL[Record](t, filepath.Join(out, "traces.jsonl")) {
		key := [2]string{r.ScenarioID, r.Controller}
		traces[key] = append(traces[key], r)
	}
	saved := readJSONL[Metrics](t, filepath.Join(out, "metrics.jsonl"))
	if len(saved) != len(cfg.Families)*2*2 {
		t.Fatalf("saved %d metric rows", len(saved))
	}
	for _, m := range saved {
		got, err := computeMetrics(scenarios[m.ScenarioID], m.Controller, traces[[2]string{m.ScenarioID, m.Controller}])
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, m) {
			t.Fatalf("recomputed %+v, saved %+v", got, m)
		}
	}
}

func readJSONL[T any](t *testing.T, path string) []T {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []T
	dec := json.NewDecoder(bytes.NewReader(data))
	for dec.More() {
		var v T
		if err := dec.Decode(&v); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	return out
}
