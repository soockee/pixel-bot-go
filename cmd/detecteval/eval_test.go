package main

import (
	"bufio"
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

	"github.com/soocke/pixel-bot-go/assets"
	"github.com/soocke/pixel-bot-go/domain/detect"
	"github.com/soocke/pixel-bot-go/internal/dataset"
	"github.com/soocke/pixel-bot-go/internal/fakegame"
)

// writeFakeDataset records two casts of the fake game, 100 ms per frame: one
// with a bite at 2000 ms, after the bobber has landed, and one without, then a
// span with no bobber.
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
	for ms := 0; ms <= 2400; ms += 100 {
		phase := fakegame.PhaseBobber
		if ms >= 2000 {
			phase = fakegame.PhaseSplash
		}
		write(ms, phase)
	}
	for ms := 3000; ms <= 3500; ms += 100 {
		write(ms, fakegame.PhaseBobber)
	}
	for ms := 4000; ms <= 4300; ms += 100 {
		write(ms, fakegame.PhaseEmpty)
	}
	b := r.BobberRect()
	biteMS := int64(2000)
	labels, err := json.Marshal(dataset.Labels{
		Casts: []dataset.Cast{
			{Span: dataset.Span{StartMS: 0, EndMS: 2400}, Bobber: dataset.Rect{X: b.Min.X, Y: b.Min.Y, W: b.Dx(), H: b.Dy()}, BiteMS: &biteMS},
			{Span: dataset.Span{StartMS: 3000, EndMS: 3500}, Bobber: dataset.Rect{X: b.Min.X, Y: b.Min.Y, W: b.Dx(), H: b.Dy()}},
		},
		Empty: []dataset.Span{{StartMS: 4000, EndMS: 4300}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "labels.json"), labels, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func evaluate(t *testing.T, locators, bites string, dirs ...string) Report {
	t.Helper()
	var out bytes.Buffer
	if err := run("", locators, bites, true, dirs, &out); err != nil {
		t.Fatal(err)
	}
	var report Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	return report
}

func TestCompareRunsEveryTechniqueAndRepeats(t *testing.T) {
	dir := writeFakeDataset(t)

	first := evaluate(t, "all", "all", dir)

	for _, name := range detect.LocatorNames() {
		want := LocatorCounts{Casts: 2, Locks: 2, LocksOnBobber: 2, EmptyFrames: 4}
		if got := first.Locators[name]; got != want {
			t.Errorf("locator %s: got %+v, want %+v", name, got, want)
		}
	}
	want := BiteCounts{Casts: 2, BiteCasts: 1, BitesCaught: 1, BiteLatenciesMS: []int64{0}}
	if got := first.Bites["motion"]; !reflect.DeepEqual(got, want) {
		t.Errorf("motion: got %+v, want %+v", got, want)
	}
	if second := evaluate(t, "all", "all", dir); !reflect.DeepEqual(first, second) {
		t.Errorf("runs differ:\n%+v\n%+v", first, second)
	}
}

func TestDatasetsAddUp(t *testing.T) {
	dir := writeFakeDataset(t)
	got := evaluate(t, "marker", "motion", dir, dir)
	if c := got.Locators["marker"]; c.Casts != 4 || c.EmptyFrames != 8 {
		t.Errorf("locator counts = %+v", c)
	}
	if c := got.Bites["motion"]; c.BiteCasts != 2 || c.BitesCaught != 2 {
		t.Errorf("bite counts = %+v", c)
	}
}

func TestPrintedReportHasOneColumnPerTechnique(t *testing.T) {
	var out bytes.Buffer
	if err := run("", "all", "all", false, []string{writeFakeDataset(t)}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"marker", "ncc", "motion", "Bobber found per cast", ">= 98%", "100.0%", "Search time p95"} {
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
		{"unknown locator", "nope", "all", []string{"x"}},
		{"unknown bite detector", "all", "nope", []string{"x"}},
		{"no datasets", "all", "all", nil},
		{"missing dataset", "all", "all", []string{"does-not-exist"}},
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

func TestWrongLockCountsAgainstTheClickRate(t *testing.T) {
	d, err := dataset.Load(writeFakeDataset(t))
	if err != nil {
		t.Fatal(err)
	}
	var got LocatorCounts

	if _, err := evaluateLocator(stubLocator{detect.Match{X: 1, Y: 1}, true}, d, &got); err != nil {
		t.Fatal(err)
	}

	if got.Locks != 2 || got.LocksOnBobber != 0 || got.FalseLocks != got.EmptyFrames {
		t.Errorf("counts = %+v", got)
	}
}

func TestBitesAreWatchedInTheMarkerBox(t *testing.T) {
	d, err := dataset.Load(writeFakeDataset(t))
	if err != nil {
		t.Fatal(err)
	}
	watches, err := watchAreas(d, 80)
	if err != nil {
		t.Fatal(err)
	}
	if got := watches[0].Area; got.Dx() == 80 || !got.Overlaps(d.Labels.Casts[0].Bobber.Image()) {
		t.Errorf("area = %v, want the marker's box", got)
	}
}

func TestScoreBite(t *testing.T) {
	bite := int64(1000)
	tests := []struct {
		name    string
		trigger int64
		bite    *int64
		want    BiteCounts
	}{
		{"on time", 1100, &bite, BiteCounts{BitesCaught: 1, BiteLatenciesMS: []int64{100}}},
		{"just before the label", 900, &bite, BiteCounts{BitesCaught: 1, BiteLatenciesMS: []int64{0}}},
		{"too early", 700, &bite, BiteCounts{FalseBites: 1}},
		{"too late", 2600, &bite, BiteCounts{}},
		{"no bite in the cast", 1000, nil, BiteCounts{FalseBites: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got BiteCounts
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
	if got := latency(BiteCounts{}); got != "n/a" {
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

func TestExportWritesCropsAndAnIndex(t *testing.T) {
	src := writeFakeDataset(t)
	out := filepath.Join(t.TempDir(), "crops")
	var status bytes.Buffer

	if err := export(out, []string{src}, 80, &status); err != nil {
		t.Fatal(err)
	}

	file, err := os.Open(filepath.Join(out, "index.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	rows := 0
	scan := bufio.NewScanner(file)
	for scan.Scan() {
		var row indexRow
		if err := json.Unmarshal(scan.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		rows++
		crop, err := os.Open(filepath.Join(out, filepath.FromSlash(row.File)))
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := png.DecodeConfig(crop)
		crop.Close()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Width != row.Box.W || cfg.Height != row.Box.H {
			t.Errorf("%s is %dx%d, box is %+v", row.File, cfg.Width, cfg.Height, row.Box)
		}
		if row.Cast == 0 && (row.BiteMS == nil || *row.BiteMS != 2000 || row.ReelMS != 2400) {
			t.Errorf("row = %+v", row)
		}
	}
	if rows != 46 { // the second cast's landing window reaches back over the first
		t.Errorf("rows = %d, want 46", rows)
	}
	if !strings.Contains(status.String(), "Exported 46 crops") {
		t.Errorf("status = %q", status.String())
	}
}

func TestExportRefusesAnExistingDirectory(t *testing.T) {
	if err := export(t.TempDir(), []string{writeFakeDataset(t)}, 80, &bytes.Buffer{}); err == nil {
		t.Error("want an error")
	}
}
