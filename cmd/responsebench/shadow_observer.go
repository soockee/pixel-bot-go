package main

import (
	"fmt"
	"strings"

	"github.com/soocke/pixel-bot-go/internal/traceobserver"
)

type shadowResult = traceobserver.Result
type shadowAlert = traceobserver.Alert
type detectorResult = traceobserver.DetectorResult

func newShadowObserver(model *shadowModel, sc Scenario) *traceobserver.Observer {
	arrivals := sc.arrivals()
	return traceobserver.New(model, arrivals[len(arrivals)-1]+sc.Environment.WindowMs+model.Config.TickMs)
}

func scoreShadowTrace(model *shadowModel, sc Scenario, trace []Record) shadowResult {
	observer := newShadowObserver(model, sc)
	for _, r := range trace {
		observer.Observe(r)
	}
	return observer.Finish()
}

type referenceResult struct {
	ScenarioID string       `json:"scenario_id"`
	Family     string       `json:"family"`
	Seed       uint64       `json:"seed"`
	Shadow     shadowResult `json:"shadow"`
}

func evaluateReference(cfg Config, split string) ([]referenceResult, error) {
	var results []referenceResult
	seeds := benchmarkSeeds(cfg, split)
	for _, family := range cfg.Families {
		for _, seed := range seeds {
			sc := generate(cfg, family, split, seed)
			trace, err := referenceTrace(sc)
			if err != nil {
				return nil, err
			}
			results = append(results, referenceResult{sc.ID, family.Name, seed, scoreShadowTrace(cfg.fittedShadow, sc, trace)})
		}
	}
	return results, nil
}

func shadowMetricDefs() []struct {
	name string
	get  func(Metrics) (float64, bool)
} {
	defs := []struct {
		name string
		get  func(Metrics) (float64, bool)
	}{}
	for _, name := range []string{"timing", "sequence"} {
		for _, stat := range []string{"flagged_run", "first_alert_ms", "episodes", "repeat_episodes", "flagged_time_rate", "scored_time_rate"} {
			defs = append(defs, struct {
				name string
				get  func(Metrics) (float64, bool)
			}{"shadow_" + name + "_" + stat, func(m Metrics) (float64, bool) {
				result := m.Shadow.Timing
				if name == "sequence" {
					result = m.Shadow.Sequence
				}
				switch stat {
				case "flagged_run":
					if result.FirstAlertMs != nil {
						return 1, true
					}
					return 0, result.ScoredTicks > 0
				case "first_alert_ms":
					return optional(result.FirstAlertMs)
				case "episodes":
					return float64(result.Episodes), result.ScoredTicks > 0
				case "repeat_episodes":
					return float64(max(0, result.Episodes-1)), result.ScoredTicks > 0
				case "flagged_time_rate":
					return float64(result.FlaggedTicks) / float64(result.Ticks), true
				default:
					return float64(result.ScoredTicks) / float64(result.Ticks), true
				}
			}})
		}
	}
	return defs
}

func shadowReport(model *shadowModel, metrics []Metrics, references []referenceResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Independent trace detectors\n\n")
	fmt.Fprintf(&b, "Reference sessions are synthetic. Target session false-alarm rate: %.1f%%. Calibration crossings: timing %d/%d, sequence %d/%d. Held-out workload families: %s.\n\n",
		100*model.Config.FalseAlarmRate, model.TimingThreshold.ReferenceAlerts, model.TimingThreshold.ReferenceRuns,
		model.SequenceThreshold.ReferenceAlerts, model.SequenceThreshold.ReferenceRuns, strings.Join(model.Config.HoldoutFamilies, ", "))
	b.WriteString("The detectors sample a common simulated-time grid. Flagged time uses all ticks, including idle or unscored time. An episode starts when a tick is flagged after a clear tick.\n\n")
	type counts struct {
		flagged, scorable, episodes int
		timeRate                    float64
	}
	type totals struct {
		timing, sequence counts
		runs             int
	}
	byController := map[string]*totals{}
	var names []string
	for _, m := range metrics {
		t := byController[m.Controller]
		if t == nil {
			t = &totals{}
			byController[m.Controller] = t
			names = append(names, m.Controller)
		}
		t.runs++
		for _, pair := range []struct {
			r detectorResult
			c *counts
		}{
			{m.Shadow.Timing, &t.timing}, {m.Shadow.Sequence, &t.sequence},
		} {
			if pair.r.ScoredTicks > 0 {
				pair.c.scorable++
			}
			if pair.r.FirstAlertMs != nil {
				pair.c.flagged++
			}
			pair.c.episodes += pair.r.Episodes
			pair.c.timeRate += float64(pair.r.FlaggedTicks) / float64(pair.r.Ticks)
		}
	}
	b.WriteString("| Profile | Timing flags | Timing episodes | Timing flagged time | Sequence flags | Sequence episodes | Sequence flagged time |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|---:|\n")
	for _, name := range names {
		t := byController[name]
		fmt.Fprintf(&b, "| %s | %d/%d | %d | %.3f%% | %d/%d | %d | %.3f%% |\n",
			name, t.timing.flagged, t.timing.scorable, t.timing.episodes, 100*t.timing.timeRate/float64(t.runs),
			t.sequence.flagged, t.sequence.scorable, t.sequence.episodes, 100*t.sequence.timeRate/float64(t.runs))
	}
	b.WriteString("\n| Synthetic reference workload | Timing false alarms | Sequence false alarms |\n")
	b.WriteString("|---|---:|---:|\n")
	type referenceCounts struct{ flagged, scorable int }
	byFamily := map[string][2]referenceCounts{}
	var families []string
	for _, ref := range references {
		counts, ok := byFamily[ref.Family]
		if !ok {
			families = append(families, ref.Family)
		}
		for i, result := range []detectorResult{ref.Shadow.Timing, ref.Shadow.Sequence} {
			if result.ScoredTicks > 0 {
				counts[i].scorable++
			}
			if result.FirstAlertMs != nil {
				counts[i].flagged++
			}
		}
		byFamily[ref.Family] = counts
	}
	for _, family := range families {
		counts := byFamily[family]
		fmt.Fprintf(&b, "| %s | %d/%d | %d/%d |\n", family,
			counts[0].flagged, counts[0].scorable, counts[1].flagged, counts[1].scorable)
	}
	return b.String()
}
