package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/soocke/pixel-bot-go/internal/playtrace"
)

func benchmarkEvent(r Record) playtrace.Event {
	var kind string
	switch r.Action {
	case actionArrive:
		kind = playtrace.BiteAvailable
	case actionAttempt:
		kind = playtrace.ReelAttempt
	case actionReply:
		kind = playtrace.ReelReply
	case actionClose:
		kind = playtrace.BiteClosed
	case "cast":
		kind = playtrace.CastDispatched
	}
	return playtrace.Event{
		Type: kind, ElapsedUS: r.TMs * 1000, RunID: r.ScenarioID + "/" + r.Controller,
		ScenarioID: r.ScenarioID, Controller: r.Controller, Seed: json.Number(strconv.FormatUint(r.Seed, 10)),
		EventID: &r.EventID, Attempt: r.Attempt, Outcome: r.Outcome,
	}
}

type manualBinding struct{ kind, value string }

func parseBinding(value string) (manualBinding, error) {
	if value == "" {
		return manualBinding{}, nil
	}
	kind, name, found := strings.Cut(value, ":")
	switch {
	case found && kind == "key" && strings.TrimSpace(name) != "":
		return manualBinding{kind, name}, nil
	case found && kind == "mouse" && slices.Contains([]string{"left", "right", "middle", "x1", "x2"}, name):
		return manualBinding{kind, name}, nil
	default:
		return manualBinding{}, fmt.Errorf("invalid binding %q, use key:Name or mouse:left|right|middle|x1|x2", value)
	}
}

func (b manualBinding) match(event playtrace.Event) (int64, bool) {
	if b.kind == "key" && event.Type == "key_down" && event.Key != nil && !event.Key.Repeat {
		key := event.Key.Name
		if event.Key.Chord != "" {
			key = event.Key.Chord
		}
		return event.ElapsedUS, strings.EqualFold(key, b.value)
	}
	if b.kind == "mouse" && event.Type == "click" && event.Button == b.value && event.Gesture != nil &&
		event.Gesture.DurationUS >= 0 && event.Gesture.DurationUS <= event.ElapsedUS {
		return event.ElapsedUS - event.Gesture.DurationUS, true
	}
	return 0, false
}

type distribution struct {
	Samples int   `json:"samples"`
	P50US   int64 `json:"p50_us"`
	P95US   int64 `json:"p95_us"`
}

func describe(values []int64) *distribution {
	if len(values) == 0 {
		return nil
	}
	return &distribution{len(values), nearestRank(values, 50), nearestRank(values, 95)}
}

type traceSummary struct {
	File            string                  `json:"file"`
	Source          string                  `json:"source"`
	Clock           string                  `json:"clock"`
	Session         string                  `json:"session_id,omitempty"`
	Controller      string                  `json:"controller,omitempty"`
	Status          string                  `json:"status"`
	Runs            int                     `json:"runs"`
	Moves           int                     `json:"moves"`
	ButtonPresses   int                     `json:"button_presses"`
	Clicks          int                     `json:"clicks"`
	Drags           int                     `json:"drags"`
	KeyPresses      int                     `json:"key_presses"`
	Casts           int                     `json:"casts"`
	BitesAvailable  int                     `json:"bites_available"`
	BitesDetected   int                     `json:"bites_detected"`
	Reels           int                     `json:"reels"`
	Completed       int                     `json:"simulated_completions"`
	ReelSpacing     *distribution           `json:"reel_spacing,omitempty"`
	InternalDelay   *distribution           `json:"internal_bite_to_reel,omitempty"`
	ClickHold       *distribution           `json:"click_hold,omitempty"`
	DragHold        *distribution           `json:"drag_hold,omitempty"`
	Activity        *activityProfile        `json:"activity,omitempty"`
	SimulatedTiming *simulatedTimingSummary `json:"simulated_timing,omitempty"`
	Gameplay        *gameplayAudit          `json:"gameplay,omitempty"`
}

type simulatedTimingSummary struct {
	Profile       string         `json:"profile"`
	Planned       int            `json:"planned"`
	Released      int            `json:"released"`
	Canceled      map[string]int `json:"canceled,omitempty"`
	Hold          *distribution  `json:"hold,omitempty"`
	ReleaseToCast *distribution  `json:"release_to_cast,omitempty"`
}

type traceAccumulator struct {
	summary                          traceSummary
	activity                         *activityTracker
	intervals, delays, clicks, drags []int64
	holds, cooldowns                 []int64
}

type runObservations struct {
	summary  *traceAccumulator
	pending  map[int]int64
	reels    []int64
	last     int64
	press    *playtrace.Event
	release  *int64
	gameplay gameplayHistory
}

func (r *runObservations) flushReels() {
	// A completed click is logged at release but represents its earlier press.
	slices.Sort(r.reels)
	for i := 1; i < len(r.reels); i++ {
		r.summary.intervals = append(r.summary.intervals, r.reels[i]-r.reels[i-1])
	}
	r.reels = r.reels[:0]
}

type loadedTrace struct {
	header            playtrace.Header
	path, status      string
	summaries         []traceSummary
	ground, responses []int64
	released          [][2]int64
	detections        []int64
	detector          string
	mixedDetectors    bool
	end               int64
}

func readTrace(path string, cast, reel manualBinding) (loadedTrace, error) {
	result := loadedTrace{path: path, status: "no footer"}
	file, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	var first json.RawMessage
	if err := decoder.Decode(&first); err != nil {
		return result, err
	}
	header, headerErr := playtrace.DecodeHeader(first)
	legacy := headerErr != nil
	if legacy {
		var row Record
		if err := json.Unmarshal(first, &row); err != nil || row.ScenarioID == "" || benchmarkEvent(row).Type == "" {
			return result, headerErr
		}
		header = playtrace.Header{Source: playtrace.Benchmark, Clock: playtrace.Virtual}
		if row.Controller == "app-fsm" && row.ScenarioID == "app-dry-run" {
			header.Source, header.Clock = playtrace.AppDryRun, playtrace.Monotonic
		}
		result.status = "legacy export"
	}
	result.header = header
	groups := map[string]*traceAccumulator{}
	runs := map[string]*runObservations{}
	var names []string

	visit := func(event playtrace.Event) error {
		if event.Type == "" || event.ElapsedUS < 0 {
			return fmt.Errorf("invalid event type or time")
		}
		if event.Type == "session_end" {
			if result.status != "no footer" || event.Status == "" {
				return fmt.Errorf("invalid or repeated trace footer")
			}
			result.status = event.Status
			result.end = max(result.end, event.ElapsedUS)
			return nil
		}
		if !legacy && result.status != "no footer" {
			return fmt.Errorf("event after trace footer")
		}
		if header.Clock == playtrace.Virtual && (event.RunID == "" || event.Controller == "") {
			return fmt.Errorf("virtual event missing run or controller")
		}
		result.end = max(result.end, event.ElapsedUS)
		group := groups[event.Controller]
		if group == nil {
			group = &traceAccumulator{summary: traceSummary{
				File: path, Source: header.Source, Clock: header.Clock, Session: header.SessionID, Controller: event.Controller,
			}}
			if header.Source == playtrace.Manual {
				group.activity = newActivityTracker(header.Keyboard)
			}
			groups[event.Controller] = group
			names = append(names, event.Controller)
		}
		run := runs[event.RunID]
		if run == nil {
			run = &runObservations{summary: group, pending: map[int]int64{}}
			runs[event.RunID] = run
			group.summary.Runs++
		}
		if run.summary != group || event.ElapsedUS < run.last {
			return fmt.Errorf("controller changed or time reversed in run %q", event.RunID)
		}
		run.last = event.ElapsedUS
		if group.activity != nil {
			group.activity.observe(event)
		}
		counts := &group.summary
		var reelTime int64
		isReel := false
		switch event.Type {
		case "segment_end", "mouse_gap":
			run.flushReels()
		case "move":
			counts.Moves++
		case "button_down":
			counts.ButtonPresses++
		case "key_down":
			if event.Key != nil && !event.Key.Repeat {
				counts.KeyPresses++
			}
		case "click":
			counts.Clicks++
			if event.Gesture != nil && event.Gesture.DurationUS >= 0 {
				group.clicks = append(group.clicks, event.Gesture.DurationUS)
			}
		case "drag":
			counts.Drags++
			if event.Gesture != nil && event.Gesture.DurationUS >= 0 {
				group.drags = append(group.drags, event.Gesture.DurationUS)
			}
		case playtrace.CastDispatched:
			counts.Casts++
			if header.Source == playtrace.AppDryRun {
				run.gameplay.cast(event.ElapsedUS)
			}
			if run.release != nil {
				group.cooldowns = append(group.cooldowns, event.ElapsedUS-*run.release)
				run.release = nil
			}
		case playtrace.TimingGap:
			run.press, run.release = nil, nil
			clear(run.pending)
			run.gameplay.gap()
		case playtrace.ReelPlanned:
			if header.Source != playtrace.AppDryRun || event.EventID == nil || event.Timing == nil ||
				event.Timing.DetectorUS < 0 || event.Timing.DetectorUS > event.ElapsedUS {
				return fmt.Errorf("invalid simulated reel plan")
			}
			if counts.SimulatedTiming == nil {
				counts.SimulatedTiming = &simulatedTimingSummary{Profile: header.TimingProfile, Canceled: map[string]int{}}
			}
			counts.SimulatedTiming.Planned++
			if len(result.detections) > 0 && result.detector != event.Timing.Detector {
				result.mixedDetectors = true
			}
			result.detector = event.Timing.Detector
			result.detections = append(result.detections, event.Timing.DetectorUS)
			run.pending[*event.EventID] = event.Timing.DetectorUS
		case playtrace.ReelReleased:
			if counts.SimulatedTiming == nil || run.press == nil || event.EventID == nil || *event.EventID != *run.press.EventID {
				return fmt.Errorf("simulated reel release has no matching press")
			}
			counts.SimulatedTiming.Released++
			group.holds = append(group.holds, event.ElapsedUS-run.press.ElapsedUS)
			result.released = append(result.released, [2]int64{run.press.ElapsedUS, event.ElapsedUS})
			run.press, run.release = nil, &event.ElapsedUS
		case playtrace.ReelCanceled:
			if counts.SimulatedTiming == nil || event.EventID == nil {
				return fmt.Errorf("simulated reel cancellation has no plan")
			}
			counts.SimulatedTiming.Canceled[event.Outcome]++
			delete(run.pending, *event.EventID)
			run.press = nil
		case playtrace.BiteAvailable:
			counts.BitesAvailable++
			if header.Source == playtrace.FakeGame {
				result.ground = append(result.ground, event.ElapsedUS)
			}
		case playtrace.BiteDetected:
			counts.BitesDetected++
		case playtrace.ReelAttempt, playtrace.ReelDispatched:
			reelTime, isReel = event.ElapsedUS, true
			if header.Source == playtrace.AppDryRun && event.Type == playtrace.ReelDispatched {
				run.gameplay.reel(reelTime)
			}
			if event.EventID != nil {
				if counts.SimulatedTiming != nil && event.Type == playtrace.ReelDispatched {
					run.press = &event
				}
				if available, ok := run.pending[*event.EventID]; ok {
					group.delays = append(group.delays, reelTime-available)
					delete(run.pending, *event.EventID)
				}
			}
		case playtrace.BiteClosed:
			if event.Outcome == outcomeCompleted {
				counts.Completed++
			}
			if event.EventID != nil {
				delete(run.pending, *event.EventID)
			}
		}
		if event.EventID != nil && (header.Source == playtrace.Benchmark && event.Type == playtrace.BiteAvailable ||
			header.Source == playtrace.AppDryRun && event.Type == playtrace.BiteDetected) {
			run.pending[*event.EventID] = event.ElapsedUS
		}
		if header.Source == playtrace.Manual {
			if _, ok := cast.match(event); ok {
				counts.Casts++
			}
			reelTime, isReel = reel.match(event)
		}
		if isReel {
			counts.Reels++
			run.reels = append(run.reels, reelTime)
			if header.Source == playtrace.AppDryRun || header.Source == playtrace.Manual {
				result.responses = append(result.responses, reelTime)
			}
		}
		return nil
	}

	if legacy {
		err = readLegacyRecords(decoder, first, header.Source, visit)
	} else {
		err = playtrace.ReadEvents(decoder, visit)
	}
	if err != nil {
		return result, err
	}
	for _, run := range runs {
		run.flushReels()
		if header.Source == playtrace.AppDryRun && run.summary.summary.Runs == 1 {
			run.summary.summary.Gameplay = run.gameplay.total.result()
		}
	}
	if len(names) == 0 {
		groups[""] = &traceAccumulator{summary: traceSummary{File: path, Source: header.Source, Clock: header.Clock, Session: header.SessionID}}
		names = append(names, "")
	}
	for _, name := range names {
		a := groups[name]
		a.summary.Status = result.status
		if a.activity != nil {
			a.summary.Activity = a.activity.summarize(header.Schema, result.status == "complete")
		}
		a.summary.ReelSpacing, a.summary.InternalDelay = describe(a.intervals), describe(a.delays)
		a.summary.ClickHold, a.summary.DragHold = describe(a.clicks), describe(a.drags)
		if timing := a.summary.SimulatedTiming; timing != nil {
			timing.Hold, timing.ReleaseToCast = describe(a.holds), describe(a.cooldowns)
		}
		result.summaries = append(result.summaries, a.summary)
	}
	return result, nil
}

func readLegacyRecords(decoder *json.Decoder, first json.RawMessage, source string, visit func(playtrace.Event) error) error {
	raw := first
	for {
		var row Record
		if err := json.Unmarshal(raw, &row); err != nil {
			return err
		}
		event := benchmarkEvent(row)
		if source == playtrace.AppDryRun {
			switch event.Type {
			case playtrace.BiteAvailable:
				event.Type = playtrace.BiteDetected
			case playtrace.ReelAttempt:
				event.Type = playtrace.ReelDispatched
			}
		}
		if err := visit(event); err != nil {
			return err
		}
		if err := decoder.Decode(&raw); errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return err
		}
	}
}

type alignedSummary struct {
	Session          string        `json:"session_id"`
	GroundFile       string        `json:"ground_file"`
	ResponseFile     string        `json:"response_file"`
	Source           string        `json:"source"`
	BitesInOverlap   int           `json:"bites_in_overlap"`
	Matched          *distribution `json:"matched_bite_to_reel,omitempty"`
	BitesWithRelease *int          `json:"bites_with_matched_reel_release,omitempty"`
	Missed           int           `json:"unmatched_bites"`
	UnmatchedReels   int           `json:"unmatched_reels"`
	TimingProfile    string        `json:"timing_profile,omitempty"`
	CursorProfile    string        `json:"cursor_profile,omitempty"`
	Detector         string        `json:"detector,omitempty"`
	DetectorDelayUS  []int64       `json:"matched_cue_to_detector_us,omitempty"`
	GroundStatus     string        `json:"ground_status"`
	ResponseStatus   string        `json:"response_status"`
}

func alignReels(ground, response loadedTrace, window time.Duration) alignedSummary {
	result := alignedSummary{Session: ground.header.SessionID, GroundFile: ground.path,
		ResponseFile: response.path, Source: response.header.Source, TimingProfile: response.header.TimingProfile,
		CursorProfile: response.header.CursorProfile,
		GroundStatus:  ground.status, ResponseStatus: response.status}
	bites, responses, delays := alignTimes(ground, response, response.responses, window)
	result.BitesInOverlap = bites
	result.Matched = describe(delays)
	result.Missed = bites - len(delays)
	result.UnmatchedReels = responses - len(delays)
	for _, summary := range response.summaries {
		if summary.SimulatedTiming == nil {
			continue
		}
		overlapEnd := min(ground.header.StartedUTC.UnixMicro()+ground.end, response.header.StartedUTC.UnixMicro()+response.end)
		var pressTimes []int64
		for _, click := range response.released {
			if response.header.StartedUTC.UnixMicro()+click[1] <= overlapEnd {
				pressTimes = append(pressTimes, click[0])
			}
		}
		_, _, released := alignTimes(ground, response, pressTimes, window)
		count := len(released)
		result.BitesWithRelease = &count
		break
	}
	if response.detector != "" && !response.mixedDetectors {
		result.Detector = response.detector
		_, _, result.DetectorDelayUS = alignTimes(ground, response, response.detections, window)
	}
	return result
}

func alignTimes(ground, response loadedTrace, times []int64, window time.Duration) (bites, responses int, delays []int64) {
	groundStart, responseStart := ground.header.StartedUTC.UnixMicro(), response.header.StartedUTC.UnixMicro()
	overlapEnd := min(groundStart+ground.end, responseStart+response.end)
	slices.Sort(times)
	next := 0
	for i, event := range ground.ground {
		at := groundStart + event
		if at < responseStart || at > overlapEnd {
			continue
		}
		bites++
		for next < len(times) && responseStart+times[next] < at {
			next++
		}
		if next == len(times) {
			continue
		}
		reply := responseStart + times[next]
		if reply > overlapEnd || reply-at > window.Microseconds() ||
			i+1 < len(ground.ground) && reply >= groundStart+ground.ground[i+1] {
			continue
		}
		delays = append(delays, reply-at)
		next++
	}
	overlapStart := max(groundStart, responseStart)
	for _, t := range times {
		if at := responseStart + t; at >= overlapStart && at <= overlapEnd {
			responses++
		}
	}
	return bites, responses, delays
}

func runTraceComparison(paths []string, castSpec, reelSpec string, window time.Duration, asJSON bool, out io.Writer) error {
	if len(paths) == 0 || window <= 0 {
		return fmt.Errorf("-compare needs trace files and a positive -match-window")
	}
	cast, err := parseBinding(castSpec)
	if err != nil {
		return err
	}
	reel, err := parseBinding(reelSpec)
	if err != nil {
		return err
	}
	if cast.kind != "" && cast == reel {
		return fmt.Errorf("manual cast and reel bindings must be distinct")
	}
	var report struct {
		ManualCastBinding string           `json:"manual_cast_binding,omitempty"`
		ManualReelBinding string           `json:"manual_reel_binding,omitempty"`
		MatchWindowUS     int64            `json:"match_window_us"`
		Traces            []traceSummary   `json:"traces"`
		Aligned           []alignedSummary `json:"aligned,omitempty"`
	}
	report.ManualCastBinding, report.ManualReelBinding = castSpec, reelSpec
	report.MatchWindowUS = window.Microseconds()
	var traces []loadedTrace
	groundBySession := map[string]loadedTrace{}
	for _, path := range paths {
		trace, err := readTrace(path, cast, reel)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		report.Traces = append(report.Traces, trace.summaries...)
		traces = append(traces, trace)
		if trace.header.Source == playtrace.FakeGame && trace.header.SessionID != "" {
			if _, exists := groundBySession[trace.header.SessionID]; exists {
				return fmt.Errorf("multiple fake-game traces for session %q", trace.header.SessionID)
			}
			groundBySession[trace.header.SessionID] = trace
		}
	}
	for _, trace := range traces {
		ground, ok := groundBySession[trace.header.SessionID]
		if ok && trace.header.StartedUTC != nil && (trace.header.Source == playtrace.AppDryRun ||
			trace.header.Source == playtrace.Manual && reel.kind != "") {
			report.Aligned = append(report.Aligned, alignReels(ground, trace, window))
		}
	}
	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	var table strings.Builder
	table.WriteString("# Trace comparison\n\n")
	if castSpec != "" || reelSpec != "" {
		fmt.Fprintf(&table, "Manual bindings | cast %s | reel %s\n\n", cell(castSpec), cell(reelSpec))
	}
	table.WriteString("| File | Source | Controller | Status | Runs | Moves | Button presses | Clicks | Drags | Keys | Casts | Bites available / detected | Reels | Simulated completions | Reel spacing p50 / p95 | Internal bite-to-reel p50 / p95 | Click hold p50 / p95 | Drag hold p50 / p95 |\n")
	table.WriteString("|---|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|---|---|---|\n")
	for _, row := range report.Traces {
		fmt.Fprintf(&table, "| %s | %s | %s | %s | %d | %d | %d | %d | %d | %d | %d | %d / %d | %d | %d | %s | %s | %s | %s |\n",
			cell(row.File), row.Source, cell(row.Controller), cell(row.Status), row.Runs, row.Moves, row.ButtonPresses, row.Clicks, row.Drags, row.KeyPresses, row.Casts,
			row.BitesAvailable, row.BitesDetected, row.Reels, row.Completed, formatDistribution(row.ReelSpacing),
			formatDistribution(row.InternalDelay), formatDistribution(row.ClickHold), formatDistribution(row.DragHold))
	}
	activityTable := false
	for _, row := range report.Traces {
		if row.Activity == nil {
			continue
		}
		if !activityTable {
			table.WriteString("\n## Manual activity profile\n\n| File | Mix / 100 | Actions | Free movement bursts | Clicks | Drags | Right drags | Drag share | Right drag time | Right drag raw travel | Plain keys | Shortcuts | Wheel packets | Actions / capture min |\n|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
			activityTable = true
		}
		p := row.Activity
		fmt.Fprintf(&table, "| %s | %s | %d | %d | %d | %d | %d | %s | %.1f ms | %.1f | %d | %d | %d | %s |\n",
			cell(row.File), formatActivityNumber(p.MixScore, 1, ""), p.Actions, p.Counts["free_move_burst"],
			p.Counts["click"], p.Counts["drag"], p.RightDrags, formatActivityNumber(p.DragShare, 100, "%"),
			float64(p.RightDragHoldUS)/1000, p.RightDragRawTravel, p.Counts["key"], p.Counts["shortcut"],
			p.Counts["wheel"], formatActivityNumber(p.ActionsPerMinute, 1, ""))
	}
	if len(report.Aligned) != 0 {
		table.WriteString("\n## Aligned fake-game bites\n\n| Session | Fake-game file | Response file | Matched / bites in overlap | With release | Unmatched reels | Delay p50 / p95 |\n|---|---|---|---:|---:|---:|---|\n")
		for _, row := range report.Aligned {
			count := 0
			if row.Matched != nil {
				count = row.Matched.Samples
			}
			released := "—"
			if row.BitesWithRelease != nil {
				released = strconv.Itoa(*row.BitesWithRelease)
			}
			fmt.Fprintf(&table, "| %s | %s | %s | %d / %d | %s | %d | %s |\n", cell(row.Session), cell(row.GroundFile),
				cell(row.ResponseFile), count, row.BitesInOverlap, released, row.UnmatchedReels, formatDistribution(row.Matched))
		}
	}
	for _, row := range report.Traces {
		if gameplay := row.Gameplay; gameplay != nil {
			fmt.Fprintf(&table, "\nGameplay for %s: %d of %d complete cast cycles had a reel, %d of %d scored motifs repeated.\n",
				cell(row.File), gameplay.ReeledCycles, gameplay.Cycles, gameplay.RepeatedMotifs, gameplay.ScoredMotifs)
		}
		if timing := row.SimulatedTiming; timing != nil {
			fmt.Fprintf(&table, "\nSimulated timing for %s (%s): %d planned, %d released, hold %s, release to cast %s.\n",
				cell(row.File), timing.Profile, timing.Planned, timing.Released, formatDistribution(timing.Hold), formatDistribution(timing.ReleaseToCast))
		}
	}
	_, err = io.WriteString(out, table.String())
	return err
}

func formatDistribution(d *distribution) string {
	if d == nil {
		return "—"
	}
	return fmt.Sprintf("%.1f / %.1f ms", float64(d.P50US)/1000, float64(d.P95US)/1000)
}

func cell(s string) string {
	return strings.NewReplacer("|", "\\|", "\n", " ", "\r", " ").Replace(s)
}

func formatActivityNumber(value *float64, scale float64, suffix string) string {
	if value == nil {
		return "—"
	}
	return fmt.Sprintf("%.1f%s", *value*scale, suffix)
}
