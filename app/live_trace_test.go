package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/debug"
	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/domain/fishing"
	"github.com/soocke/pixel-bot-go/internal/playtrace"
	"github.com/soocke/pixel-bot-go/internal/traceobserver"
	uimodel "github.com/soocke/pixel-bot-go/ui/model"
)

func TestDryRunTraceAppearsInDebugState(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		model := &traceobserver.Model{
			Config:            traceobserver.Config{TickMs: 100, ScoreAgeMs: 1000},
			TimingThreshold:   traceobserver.Threshold{Cutoff: .1},
			SequenceThreshold: traceobserver.Threshold{Cutoff: .1},
		}
		trace := newLiveTrace(model, nil, time.Now())
		press, move, click := action.DryRun(slog.New(slog.NewTextHandler(io.Discard, nil)))
		fsm := fishing.NewFSM(nil, config.DefaultConfig(), fishing.ActionCallbacks{
			PressKey: press, MoveCursor: move, ClickRight: click, ParseVK: func(string) (byte, bool) { return 0x72, true },
			ObserveCast: trace.cast, ObserveReel: trace.bite,
		})
		defer fsm.Close()
		a := &app{container: &AppContainer{Capture: &uimodel.CaptureModel{}, FSM: fsm, trace: trace}, start: time.Now()}
		handler := debug.StateHandler(a.stateSnapshot)

		fsm.EventAwaitFocus()
		fsm.EventFocusAcquired()
		for range 24 {
			fsm.ForceCast()
			fsm.EventTargetAcquiredAt(10, 10)
			fsm.EventFishBite()
			synctest.Wait()
			time.Sleep(400 * time.Millisecond)
			synctest.Wait()
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, debug.StatePath, nil))
		var state struct {
			FSMState string            `json:"fsm_state"`
			Trace    liveTraceSnapshot `json:"trace_observer"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK || state.FSMState != "cooldown" ||
			state.Trace.Result.Timing.Episodes == 0 || state.Trace.Result.Sequence.Episodes == 0 {
			t.Fatalf("expected live FSM state and both detector summaries: %s", response.Body)
		}
		for _, alert := range state.Trace.RecentAlerts {
			if alert.Trigger.Outcome != "" && alert.Trigger.Outcome != traceobserver.OutcomeDryRunDispatched {
				t.Fatalf("unexpected trigger outcome: %+v", alert)
			}
		}
		if len(state.Trace.RecentAlerts) < 2 {
			t.Fatalf("expected alert evidence: %s", response.Body)
		}
		fsm.EventHalt()
		synctest.Wait()
	})
}

func TestDryRunExportsCorrelatedEventsWithoutDetectorModel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.jsonl")
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		utc := start.UTC()
		output, err := playtrace.Create(path, playtrace.Header{
			Source: playtrace.AppDryRun, Clock: playtrace.Monotonic, StartedUTC: &utc, SessionID: "fixture",
		})
		if err != nil {
			t.Fatal(err)
		}
		trace := newLiveTrace(nil, output, start)
		press, move, click := action.DryRun(slog.New(slog.NewTextHandler(io.Discard, nil)))
		fsm := fishing.NewFSM(nil, config.DefaultConfig(), fishing.ActionCallbacks{
			PressKey: press, MoveCursor: move, ClickRight: click, ParseVK: func(string) (byte, bool) { return 0x72, true },
			ObserveCast: trace.cast, ObserveReel: trace.bite,
			PlanReel:          func(string) fishing.ReelTiming { return fishing.ReelTiming{Hold: 60 * time.Millisecond} },
			ObserveReelTiming: trace.reelTiming,
		})
		defer fsm.Close()
		fsm.EventAwaitFocus()
		fsm.EventFocusAcquired()
		fsm.ForceCast()
		synctest.Wait()
		fsm.EventTargetAcquiredAt(10, 10)
		fsm.EventFishBite()
		synctest.Wait()
		time.Sleep(400 * time.Millisecond)
		fsm.EventHalt()
		synctest.Wait()
		if err := output.Close(playtrace.End{ElapsedUS: time.Since(start).Microseconds(), Status: "complete"}); err != nil {
			t.Fatal(err)
		}
	})
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		t.Fatal(err)
	}
	header, err := playtrace.DecodeHeader(raw)
	if err != nil || header.Source != playtrace.AppDryRun || header.SessionID != "fixture" {
		t.Fatalf("header = %+v, err = %v", header, err)
	}
	var rows []playtrace.Event
	if err := playtrace.ReadEvents(decoder, func(e playtrace.Event) error {
		rows = append(rows, e)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{playtrace.CastRequested, playtrace.CastDispatched, playtrace.BiteDetected,
		playtrace.ReelPlanned, playtrace.ReelDispatched, playtrace.ReelReleased, "session_end"}
	if len(rows) != len(want) {
		t.Fatalf("events = %+v", rows)
	}
	for i, row := range rows {
		if row.Type != want[i] || i < len(rows)-1 && (row.EventID == nil || *row.EventID != 0) {
			t.Fatalf("unexpected event %d: %+v", i, row)
		}
	}
	if plan := rows[3].Timing; plan == nil || plan.HoldUS != 60_000 || plan.DelayUS != 300_000 || plan.CooldownUS != 8_000_000 {
		t.Fatalf("unexpected reel plan: %+v", plan)
	}
	if rows[4].ElapsedUS-rows[2].ElapsedUS != 300_000 || rows[4].Outcome != traceobserver.OutcomeDryRunDispatched ||
		rows[5].ElapsedUS-rows[4].ElapsedUS != 60_000 {
		t.Fatalf("unexpected reel dispatch or release: %+v", rows[4:6])
	}
}
