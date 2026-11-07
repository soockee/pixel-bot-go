package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/assets"
	"github.com/soocke/pixel-bot-go/domain/detect"
	"github.com/soocke/pixel-bot-go/internal/dataset"
	"github.com/soocke/pixel-bot-go/internal/fakegame"
)

// writeFakeDataset records two casts of the fake game, 100 ms per frame: one
// with a bite at 1000 ms and one without, then a span with no bobber.
func writeFakeDataset(t *testing.T) string {
	t.Helper()
	tmpl, err := assets.FishingTargetImage()
	if err != nil {
		t.Fatal(err)
	}
	r := fakegame.NewRenderer(fakegame.DefaultScenario(), tmpl)
	dir := filepath.Join(t.TempDir(), "fake")
	if err := os.MkdirAll(filepath.Join(dir, "frames"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(ms int, phase fakegame.Phase) {
		file, err := os.Create(filepath.Join(dir, "frames", fmt.Sprintf("%09d.png", ms)))
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, r.Render(phase)); err != nil {
			t.Fatal(err)
		}
	}
	for ms := 0; ms <= 1200; ms += 100 {
		phase := fakegame.PhaseBobber
		if ms >= 1000 {
			phase = fakegame.PhaseSplash
		}
		write(ms, phase)
	}
	for ms := 2000; ms <= 2500; ms += 100 {
		write(ms, fakegame.PhaseBobber)
	}
	for ms := 3000; ms <= 3300; ms += 100 {
		write(ms, fakegame.PhaseEmpty)
	}
	b := r.BobberRect()
	biteMS := int64(1000)
	labels, err := json.Marshal(dataset.Labels{
		Casts: []dataset.Cast{
			{Span: dataset.Span{StartMS: 0, EndMS: 1200}, Bobber: dataset.Rect{X: b.Min.X, Y: b.Min.Y, W: b.Dx(), H: b.Dy()}, BiteMS: &biteMS},
			{Span: dataset.Span{StartMS: 2000, EndMS: 2500}, Bobber: dataset.Rect{X: b.Min.X, Y: b.Min.Y, W: b.Dx(), H: b.Dy()}},
		},
		Empty: []dataset.Span{{StartMS: 3000, EndMS: 3300}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "labels.json"), labels, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRunMeasuresTheRealDetectorsAndRepeats(t *testing.T) {
	dir := writeFakeDataset(t)
	evaluate := func() []row {
		var out bytes.Buffer
		if err := run("", "ncc", "motion", true, []string{dir}, &out); err != nil {
			t.Fatal(err)
		}
		var rows []row
		if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		return rows
	}

	first := evaluate()

	want := Counts{Casts: 2, Locks: 2, LocksOnBobber: 2, EmptyFrames: 4, BiteCasts: 1, BitesCaught: 1, BiteLatenciesMS: []int64{0}}
	if got := first[len(first)-1].Counts; !reflect.DeepEqual(got, want) {
		t.Errorf("counts = %+v, want %+v", got, want)
	}
	if second := evaluate(); !reflect.DeepEqual(first, second) {
		t.Errorf("runs differ:\n%+v\n%+v", first, second)
	}
}

func TestPrintedTableShowsTargets(t *testing.T) {
	var out bytes.Buffer
	if err := run("", "ncc", "motion", false, []string{writeFakeDataset(t)}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Bobber found per cast", ">= 98%", "100.0%", "Search time p95"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}

func TestRunRejectsBadArguments(t *testing.T) {
	tests := []struct {
		name          string
		locator, bite string
		dirs          []string
	}{
		{"unknown locator", "nope", "motion", []string{"x"}},
		{"unknown bite detector", "ncc", "nope", []string{"x"}},
		{"no datasets", "ncc", "motion", nil},
		{"missing dataset", "ncc", "motion", []string{"does-not-exist"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := run("", tt.locator, tt.bite, false, tt.dirs, &bytes.Buffer{}); err == nil {
				t.Error("want an error")
			}
		})
	}
}

type stubLocator struct {
	match detect.Match
	found bool
}

func (s stubLocator) Locate(*image.RGBA) (detect.Match, bool) { return s.match, s.found }

type stubBite struct{ atMS int64 }

func (s stubBite) Reset() {}
func (s stubBite) Feed(_ *image.RGBA, t time.Time) bool {
	return s.atMS >= 0 && t.UnixMilli() == s.atMS
}

func TestWrongLockCountsAgainstTheClickRate(t *testing.T) {
	d, err := dataset.Load(writeFakeDataset(t))
	if err != nil {
		t.Fatal(err)
	}
	eval := &evaluator{locator: stubLocator{detect.Match{X: 1, Y: 1}, true}, bite: stubBite{-1}, roiSize: 40}

	got, err := eval.run(d)
	if err != nil {
		t.Fatal(err)
	}

	if got.Locks != 2 || got.LocksOnBobber != 0 || got.FalseLocks != got.EmptyFrames {
		t.Errorf("counts = %+v", got)
	}
}

func TestScoreBite(t *testing.T) {
	bite := int64(1000)
	tests := []struct {
		name    string
		trigger int64
		bite    *int64
		want    Counts
	}{
		{"on time", 1100, &bite, Counts{BitesCaught: 1, BiteLatenciesMS: []int64{100}}},
		{"just before the label", 900, &bite, Counts{BitesCaught: 1, BiteLatenciesMS: []int64{0}}},
		{"too early", 700, &bite, Counts{FalseBites: 1}},
		{"too late", 2600, &bite, Counts{}},
		{"no bite in the cast", 1000, nil, Counts{FalseBites: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Counts
			scoreBite(tt.trigger, tt.bite, &got)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestEmptyMeasurementsAreNotReportedAsZero(t *testing.T) {
	if got := percent(0, 0); got != "n/a" {
		t.Errorf("percent(0, 0) = %q", got)
	}
	if got := latency(Counts{}); got != "n/a" {
		t.Errorf("latency = %q", got)
	}
}

func TestPercentile95(t *testing.T) {
	var hundred []int64
	for i := range 100 {
		hundred = append(hundred, int64(100-i))
	}
	tests := []struct {
		values []int64
		want   int64
	}{
		{nil, 0},
		{[]int64{7}, 7},
		{[]int64{1, 2}, 2},
		{hundred, 95},
	}
	for _, tt := range tests {
		if got := percentile95(tt.values); got != tt.want {
			t.Errorf("percentile95(%d values) = %d, want %d", len(tt.values), got, tt.want)
		}
	}
}
