package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/internal/playtrace"
)

func writeComparisonFixture(t *testing.T, path string, h playtrace.Header, events []playtrace.Event, end int64) {
	t.Helper()
	w, err := playtrace.Create(path, h)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if err := w.Write(e); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(playtrace.End{ElapsedUS: end, Status: "complete"}); err != nil {
		t.Fatal(err)
	}
}

func TestCompareFourTraceSources(t *testing.T) {
	dir := t.TempDir()
	ground := filepath.Join(dir, "game.jsonl")
	manual := filepath.Join(dir, "manual.jsonl")
	app := filepath.Join(dir, "app.jsonl")
	bench := filepath.Join(dir, "bench.jsonl")
	start := time.Unix(10000, 0).UTC()
	later := start.Add(500 * time.Millisecond)
	id := 0
	writeComparisonFixture(t, ground, playtrace.Header{Source: playtrace.FakeGame, Clock: playtrace.Monotonic, SessionID: "A", StartedUTC: &start},
		[]playtrace.Event{{Type: playtrace.BiteAvailable, ElapsedUS: 1_000_000, EventID: &id},
			{Type: playtrace.BiteAvailable, ElapsedUS: 2_000_000, EventID: &id}}, 3_000_000)
	writeComparisonFixture(t, manual, playtrace.Header{Source: playtrace.Manual, Clock: playtrace.Monotonic, SessionID: "A", StartedUTC: &later},
		[]playtrace.Event{
			{Type: "click", ElapsedUS: 300_000, Button: "x1", Gesture: &playtrace.Gesture{DurationUS: 40_000}},
			{Type: "click", ElapsedUS: 850_000, Button: "right", Gesture: &playtrace.Gesture{DurationUS: 50_000}},
			{Type: "drag", ElapsedUS: 1_800_000, Button: "right", Gesture: &playtrace.Gesture{DurationUS: 90_000}},
		}, 2_500_000)
	writeComparisonFixture(t, app, playtrace.Header{Source: playtrace.AppDryRun, Clock: playtrace.Monotonic, SessionID: "A", StartedUTC: &later},
		[]playtrace.Event{{Type: playtrace.BiteDetected, ElapsedUS: 600_000, EventID: &id},
			{Type: playtrace.ReelDispatched, ElapsedUS: 900_000, EventID: &id}}, 2_500_000)
	var events []playtrace.Event
	for _, r := range []Record{
		{ScenarioID: "sc1", Controller: "base", Action: actionArrive, TMs: 10},
		{ScenarioID: "sc1", Controller: "base", Action: actionAttempt, TMs: 35},
		{ScenarioID: "sc1", Controller: "base", Action: actionClose, TMs: 50, Outcome: outcomeCompleted},
		{ScenarioID: "sc2", Controller: "base", Action: actionArrive, TMs: 2},
		{ScenarioID: "sc2", Controller: "base", Action: actionAttempt, TMs: 27},
	} {
		events = append(events, benchmarkEvent(r))
	}
	writeComparisonFixture(t, bench, playtrace.Header{Source: playtrace.Benchmark, Clock: playtrace.Virtual}, events, 0)
	var output bytes.Buffer
	if err := runTraceComparison([]string{ground, manual, app, bench}, "mouse:x1", "mouse:right", time.Second, true, &output); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Traces  []traceSummary
		Aligned []alignedSummary
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Traces) != 4 || len(result.Aligned) != 2 {
		t.Fatalf("comparison: %s", output.String())
	}
	m, a, b := result.Traces[1], result.Traces[2], result.Traces[3]
	if m.Casts != 1 || m.Reels != 1 || m.Drags != 1 || a.Completed != 0 || a.InternalDelay.P50US != 300_000 ||
		b.Runs != 2 || b.Completed != 1 || b.ReelSpacing != nil || b.InternalDelay.P50US != 25_000 {
		t.Fatalf("unexpected source metrics: %s", output.String())
	}
	if result.Aligned[0].Matched.P50US != 300_000 || result.Aligned[1].Matched.P50US != 400_000 ||
		result.Aligned[0].BitesInOverlap != 2 || result.Aligned[0].Matched.Samples != 1 {
		t.Fatalf("unexpected time alignment: %s", output.String())
	}
}

func TestCompareReadsLegacyRecorderAndBenchmark(t *testing.T) {
	dir := t.TempDir()
	manual := filepath.Join(dir, "manual-v2.jsonl")
	bench := filepath.Join(dir, "legacy-bench.jsonl")
	if err := os.WriteFile(manual, []byte(`{"type":"session_start","schema":2}
{"type":"click","t_us":1500,"button":"x1","gesture":{"duration_us":500}}
{"type":"session_end","t_us":2000,"status":"complete"}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bench, []byte(`{"scenario_id":"eval-1","controller":"base","action":"arrive","t_ms":10,"event_id":0}
{"scenario_id":"eval-1","controller":"base","action":"attempt","t_ms":25,"event_id":0}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cast, _ := parseBinding("mouse:x1")
	m, err := readTrace(manual, cast, manualBinding{})
	if err != nil || m.header.Source != playtrace.Manual || m.summaries[0].Casts != 1 {
		t.Fatalf("legacy recorder = %+v, %v", m, err)
	}
	b, err := readTrace(bench, manualBinding{}, manualBinding{})
	if err != nil || b.header.Source != playtrace.Benchmark || b.summaries[0].InternalDelay.P50US != 15_000 {
		t.Fatalf("legacy benchmark = %+v, %v", b, err)
	}
}
