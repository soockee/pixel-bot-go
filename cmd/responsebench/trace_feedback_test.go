package main

import (
	"bytes"
	"encoding/json"
	"image"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/soocke/pixel-bot-go/assets"
	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/domain/detect"
	"github.com/soocke/pixel-bot-go/domain/fishing"
	"github.com/soocke/pixel-bot-go/internal/fakegame"
	"github.com/soocke/pixel-bot-go/internal/traceobserver"
)

func TestTraceScorerSignalsOnKnownPatterns(t *testing.T) {
	for _, tc := range []struct {
		name        string
		times       []int64
		attempts    []int
		retryWindow int64
		want        string
	}{
		{"regular", []int64{0, 25, 50, 75, 100, 125}, []int{1, 1, 1, 1, 1, 1}, 250, featureTiming},
		{"motif", []int64{0, 10, 50, 110}, []int{1, 2, 1, 2}, 1, featureSequence},
		{"burst", []int64{0, 10, 50, 120}, []int{1, 2, 2, 2}, 250, featureRetries},
		{"irregular", []int64{0, 30, 100, 160, 280}, []int{1, 1, 1, 1, 1}, 250, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := *testConfig(t).TraceFeedback
			cfg.RetryWindowMs = tc.retryWindow
			scorer := traceScorer{cfg: cfg}
			for i, now := range tc.times {
				scorer.observe(Record{TMs: now, EventID: i, Action: actionAttempt, Attempt: tc.attempts[i]}, nil)
			}
			signals := scorer.result.Signals
			if tc.want == "" {
				if len(signals) != 0 || !scorer.result.Scorable {
					t.Fatalf("expected a scorable, unflagged trace: %+v", scorer.result)
				}
				return
			}
			if len(signals) != 1 || signals[0].Feature != tc.want || len(signals[0].EventIDs) == 0 ||
				signals[0].Score < cfg.Threshold || signals[0].Adjustment != nil {
				t.Fatalf("signals = %+v, want one %s signal", signals, tc.want)
			}
		})
	}
}

func TestFeedbackOnlyChangesAttemptsAfterFirstSignal(t *testing.T) {
	sc := manualScenario([]int{0}, Fault{StartMs: 0, EndMs: 120, Kind: faultFail})
	sc.Environment.WindowMs = 500
	cc := ControllerConfig{Name: "test-backoff", Kind: kindBackoff, BaseBackoffMs: 10, MaxBackoffMs: 200, MaxAttempts: 6}
	cfg := *testConfig(t).TraceFeedback
	cfg.RetryBurstCount = 2

	unobserved := mustSim(t, sc, newController(cc, sc.Seed))
	off, offScore, err := simulateWithFeedback(sc, cc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for i := range unobserved {
		unobserved[i].Controller = cc.Name
	}
	if !reflect.DeepEqual(off, unobserved) {
		t.Fatal("scoring alone changed the trace")
	}
	cc.Name += feedbackSuffix
	on, onScore, err := simulateWithFeedback(sc, cc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(onScore.Signals) != 1 || offScore.FirstSignalMs == nil ||
		*offScore.FirstSignalMs != onScore.Signals[0].TMs {
		t.Fatalf("first crossings differed: off %+v, on %+v", offScore, onScore)
	}
	signal := onScore.Signals[0]
	if signal.Feature != featureRetries || signal.Adjustment == nil || signal.Adjustment.Scope != "retries" {
		t.Fatalf("expected a retry adjustment: %+v", signal)
	}
	var before, after []int64
	for _, r := range off {
		if r.Action == actionAttempt {
			before = append(before, r.TMs)
		}
	}
	for _, r := range on {
		if r.Action == actionAttempt {
			after = append(after, r.TMs)
		}
	}
	if len(after) != 4 || len(before) < 4 || !slices.Equal(before[:3], after[:3]) ||
		after[2] != signal.TMs || after[3] != signal.Adjustment.UntilMs || before[3] >= after[3] {
		t.Fatalf("attempts before %v, after %v, signal %+v", before, after, signal)
	}
	replay, replayScore, err := simulateWithFeedback(sc, cc, cfg)
	if err != nil || !reflect.DeepEqual(on, replay) || !reflect.DeepEqual(onScore, replayScore) {
		t.Fatal("feedback replay was not deterministic")
	}
}

func TestFeedbackRunUsesThreeProfilesAndSelectedSeeds(t *testing.T) {
	out := filepath.Join(t.TempDir(), "eval")
	if err := runExperiment(out, "eval", "", &bytes.Buffer{}, true, "200001,200000"); err != nil {
		t.Fatal(err)
	}
	metrics := readJSONL[Metrics](t, filepath.Join(out, "metrics.jsonl"))
	if len(metrics) != 3*2*len(testConfig(t).Families) {
		t.Fatalf("got %d metrics rows", len(metrics))
	}
	for start := 0; start < len(metrics); start += 3 {
		a, b, c := metrics[start], metrics[start+1], metrics[start+2]
		if a.Seed != 200000+uint64(start/3%2) || a.ScenarioID != b.ScenarioID || b.ScenarioID != c.ScenarioID ||
			a.TraceScore == nil || b.TraceScore == nil || c.TraceScore == nil ||
			a.Shadow == nil || b.Shadow == nil || c.Shadow == nil ||
			a.Shadow.Timing.Ticks != b.Shadow.Timing.Ticks || b.Shadow.Timing.Ticks != c.Shadow.Timing.Ticks ||
			c.Controller != b.Controller+feedbackSuffix ||
			!reflect.DeepEqual(b.TraceScore.FirstSignalMs, c.TraceScore.FirstSignalMs) {
			t.Fatalf("unexpected profile group: %+v", metrics[start:start+3])
		}
	}
	references := readJSONL[referenceResult](t, filepath.Join(out, "shadow-reference.jsonl"))
	if len(references)*3 != len(metrics) {
		t.Fatalf("got %d reference runs for %d profile runs", len(references), len(metrics))
	}
	if _, err := parseSeeds("100000", testConfig(t).Splits["eval"]); err == nil {
		t.Fatal("accepted a development seed for eval")
	}
}

const (
	replayCast       = "cast"
	replayDispatched = traceobserver.OutcomeDryRunDispatched
)

func TestFakeGameReplayFeedsDryRunAttemptsToScorer(t *testing.T) {
	template, err := assets.FishingTargetImage()
	if err != nil {
		t.Fatal(err)
	}
	fixture := fakegame.DefaultScenario()
	renderer := fakegame.NewRenderer(fixture, template)
	target := renderer.BobberRect()
	center := target.Min.Add(image.Pt(target.Dx()/2, target.Dy()/2))
	roi := image.Rect(center.X-40, center.Y-40, center.X+40, center.Y+40)
	quiet := renderer.Render(fakegame.PhaseBobber).SubImage(roi).(*image.RGBA)
	splash := renderer.Render(fakegame.PhaseSplash).SubImage(roi).(*image.RGBA)
	cfg := testConfig(t)
	model, err := fitShadow(cfg)
	if err != nil {
		t.Fatal(err)
	}

	const casts, quietFrames, splashFrames = 24, 70, 3 // 3.5 s of quiet frames: a silent game's splash counts after 3 s
	const frameInterval, settle = 50 * time.Millisecond, 350 * time.Millisecond
	duration := casts * ((quietFrames+splashFrames)*frameInterval + settle)
	observer := traceobserver.New(model, duration.Milliseconds())
	observer.EnableEvidence(0)
	scorer := traceScorer{cfg: *cfg.TraceFeedback}
	var trace []Record
	var result shadowResult
	synctest.Test(t, func(t *testing.T) {
		var logged bytes.Buffer
		press, move, click := action.DryRun(slog.New(slog.NewJSONHandler(&logged, nil)))
		start := time.Now()
		castID, reeled := 0, false
		emit := func(kind string, attempt int, outcome string) {
			r := Record{
				ScenarioID: "app-dry-run", Seed: uint64(fixture.Seed), Controller: "app-fsm",
				TMs: time.Since(start).Milliseconds(), EventID: castID,
				Action: kind, Attempt: attempt, Outcome: outcome,
			}
			trace = append(trace, r)
			scorer.observe(r, nil)
			observer.Observe(r)
		}
		fsm := fishing.NewFSM(nil, config.DefaultConfig(), fishing.ActionCallbacks{
			PressKey: func(vk byte) bool {
				press(vk)
				emit(replayCast, 0, replayDispatched)
				return true
			},
			MoveCursor: move,
			ParseVK:    func(string) (byte, bool) { return 0x72, true },
			ClickRight: func() bool {
				click()
				emit(actionAttempt, 1, "")
				// A dry-run callback is observable. A real server reply is not.
				emit(actionReply, 1, replayDispatched)
				emit(actionClose, 1, replayDispatched)
				reeled = true
				return true
			},
		})
		defer fsm.Close()
		bite := detect.NewSplashSoundDetector(nil)
		feed := func(frame *image.RGBA) {
			if bite.Feed(frame, time.Now()) {
				fsm.EventFishBite()
			}
		}
		for id := range casts {
			castID, reeled = id, false
			bite.Reset()
			fsm.EventAwaitFocus()
			fsm.EventFocusAcquired()
			fsm.ForceCast()
			fsm.EventTargetAcquiredAt(center.X, center.Y)
			synctest.Wait()
			for range quietFrames {
				feed(quiet)
				time.Sleep(frameInterval)
			}
			emit(actionArrive, 0, "")
			for range splashFrames {
				feed(splash)
				time.Sleep(frameInterval)
			}
			time.Sleep(settle)
			fsm.EventHalt()
			synctest.Wait()
			if !reeled {
				emit(actionClose, 0, outcomeMissed)
			}
		}
		if got := bytes.Count(logged.Bytes(), []byte(`"action":"click_right"`)); got != casts {
			t.Fatalf("dry-run reel count = %d, want %d", got, casts)
		}
		if got := bytes.Count(logged.Bytes(), []byte(`"action":"press_key"`)); got != casts {
			t.Fatalf("dry-run cast count = %d, want %d", got, casts)
		}
		signals := scorer.result.Signals
		if len(signals) != 1 || signals[0].Feature != featureTiming ||
			!slices.Equal(signals[0].EventIDs, []int{0, 1, 2, 3, 4}) {
			t.Fatalf("replay signals = %+v, want one timing crossing for five casts", signals)
		}
		result = observer.Finish()
	})
	if result.Timing.ScoredTicks == 0 || result.Sequence.ScoredTicks == 0 {
		t.Fatalf("expected both observers to score the real replay: %+v", result)
	}
	for i, r := range trace {
		want := []string{replayCast, actionArrive, actionAttempt, actionReply, actionClose}
		if r.EventID != i/len(want) || r.Action != want[i%len(want)] || i > 0 && r.TMs < trace[i-1].TMs {
			t.Fatalf("invalid cast correlation or trace ordering at record %d: %+v", i, r)
		}
	}
	if len(trace) != casts*5 {
		t.Fatalf("got %d records, want %d", len(trace), casts*5)
	}
	verifyReplayEvidence(t, model, duration, trace)
	t.Logf("dry-run replay: %d casts, %d timing and %d sequence alert episodes",
		casts, result.Timing.Episodes, result.Sequence.Episodes)
	if out := os.Getenv("RESPONSEBENCH_REPLAY_OUT"); out != "" {
		if err := saveReplayEvidence(out, model, result, trace, observer.Alerts()); err != nil {
			t.Fatal(err)
		}
		t.Logf("replay evidence saved to %s", out)
	}
}

func verifyReplayEvidence(t *testing.T, model *shadowModel, duration time.Duration, trace []Record) {
	t.Helper()
	// Force both detectors to alert so this checks evidence even if a future
	// calibration classifies this particular fixture as routine.
	probe := *model
	probe.TimingThreshold.Cutoff, probe.SequenceThreshold.Cutoff = -1, -1
	observer := traceobserver.New(&probe, duration.Milliseconds())
	observer.EnableEvidence(0)
	for _, r := range trace {
		observer.Observe(r)
	}
	result := observer.Finish()
	alerts := observer.Alerts()
	if len(alerts) != result.Timing.Episodes+result.Sequence.Episodes ||
		result.Timing.Episodes == 0 || result.Sequence.Episodes == 0 {
		t.Fatalf("alert evidence does not cover both detectors: %+v", alerts)
	}
	for _, alert := range alerts {
		if alert.Trigger.TMs > alert.TMs || alert.Score <= alert.Threshold ||
			!slices.Contains(trace, alert.Trigger) || !slices.Contains(alert.CastIDs, alert.Trigger.EventID) {
			t.Fatalf("invalid alert evidence: %+v", alert)
		}
	}
}

func saveReplayEvidence(out string, model *shadowModel, result shadowResult, trace []Record, alerts []shadowAlert) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	if err := os.Mkdir(out, 0o755); err != nil {
		return err
	}
	if err := writeJSONL(filepath.Join(out, "dry-run-trace.jsonl"), trace); err != nil {
		return err
	}
	if err := writeJSONL(filepath.Join(out, "dry-run-alerts.jsonl"), alerts); err != nil {
		return err
	}
	for _, file := range []struct {
		name string
		data any
	}{
		{"shadow-model.json", model}, {"dry-run-summary.json", result},
	} {
		data, err := json.MarshalIndent(file.data, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, file.name), append(data, '\n'), 0o644); err != nil {
			return err
		}
	}
	return writeChecksums(out, []string{"dry-run-trace.jsonl", "dry-run-alerts.jsonl", "dry-run-summary.json", "shadow-model.json"})
}
