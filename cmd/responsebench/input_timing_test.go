package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/internal/inputtiming"
	"github.com/soocke/pixel-bot-go/internal/playtrace"
)

func timingFixture(t *testing.T, root, name string, holdUS int64, reviewed bool) string {
	t.Helper()
	dir := filepath.Join(root, name)
	start := time.Now().UTC()
	w, err := playtrace.Create(filepath.Join(dir, "input.jsonl"), playtrace.Header{
		Source: playtrace.Manual, Clock: playtrace.Monotonic, StartedUTC: &start,
	})
	if err != nil {
		t.Fatal(err)
	}
	click := func(button string, press, hold int64) playtrace.Event {
		return playtrace.Event{Type: "click", Button: button, ElapsedUS: press + hold,
			Gesture: &playtrace.Gesture{DurationUS: hold}}
	}
	for _, event := range []playtrace.Event{
		{Type: "segment_start"},
		click("x1", 1_000_000, 100_000),
		click("right", 5_000_000, holdUS),
		click("x1", 8_000_000, 100_000),
		{Type: "mouse_gap", ElapsedUS: 9_000_000},
		click("right", 10_000_000, 120_000),
		{Type: "segment_end", ElapsedUS: 11_000_000},
	} {
		if err := w.Write(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(playtrace.End{ElapsedUS: 11_000_000, Status: "complete"}); err != nil {
		t.Fatal(err)
	}
	if reviewed {
		review := []byte(`{"cast_binding":"mouse:x1","reel_binding":"mouse:right","casts":[
			{"cast_us":1000000,"outcome":"reeled","reel_us":5000000,
			 "cue":{"earliest_us":4500000,"latest_us":4600000,"source":"visual"}},
			{"cast_us":8000000,"outcome":"no_bite"}
		]}`)
		if err := os.WriteFile(filepath.Join(dir, reviewFile), review, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestInputTimingFitUsesReviewedTrainingSessionsOnly(t *testing.T) {
	root := t.TempDir()
	train := timingFixture(t, root, "train", 140_000, true)
	holdout := timingFixture(t, root, "holdout", 240_000, true)
	unreviewed := timingFixture(t, root, "unreviewed", 190_000, false)
	path := filepath.Join(root, "model.json")
	calPath := filepath.Join(root, "calibration.json")
	if err := os.WriteFile(calPath, []byte(`{"aligned":[{"session_id":"dev",
		"source":"app_dry_run","timing_profile":"fixed","detector":"motion",
		"ground_status":"complete","response_status":"complete",
		"matched_cue_to_detector_us":[50000,60000,70000,80000,90000]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	calibration, err := readTimingCalibration(calPath)
	if err != nil {
		t.Fatal(err)
	}
	var report bytes.Buffer
	err = runInputTiming([]string{train, holdout, unreviewed}, "mouse:x1", "mouse:right", "train,unreviewed", path, calibration, &report)
	if err != nil {
		t.Fatal(err)
	}
	model, err := inputtiming.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Samples) != 1 || len(model.Holdout) != 1 || model.Holdout[0].Name != "holdout" {
		t.Fatalf("unexpected model: %+v", model)
	}
	sample := model.Sampler(5)()
	if sample.HoldUS != 140_000 || sample.CooldownUS != 2_860_000 || sample.ReactionUS == nil ||
		*sample.ReactionUS != [2]int64{400_000, 500_000} ||
		model.Calibration.DelayUS("motion", sample) != 380_000 || model.Calibration.DelayUS("splash", sample) != 0 {
		t.Fatalf("unexpected sample: %+v", sample)
	}
	var got struct{ Sessions []timingSession }
	if err := json.Unmarshal(report.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	casts := got.Sessions[0].Casts
	if len(casts) != 2 || len(casts[1].ReelCandidates) != 0 || casts[0].ReactionUS == nil ||
		*casts[0].ReactionUS != [2]int64{400_000, 500_000} {
		t.Fatalf("unexpected reviewed cast intervals: %+v", casts)
	}
}
