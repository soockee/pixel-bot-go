// Command detecteval measures a bobber locator and a bite detector on recorded
// datasets. It prints the metrics from docs/ARCHITECTURE_REVIEW.md with their
// targets.
//
// Usage: detecteval [-config file] [-locator name] [-bite name] [-json] dataset-dir...
//
//	detecteval -label dataset-dir [-cast binding] [-reel binding] [-reaction ms] [-box px]
//
// -label writes labels.json from a session's cast and reel input.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"io"
	"log/slog"
	"maps"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/soocke/pixel-bot-go/assets"
	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/detect"
	"github.com/soocke/pixel-bot-go/internal/dataset"
)

// Candidates under comparison. Add new detectors here.
var (
	locators = map[string]func(tmpl image.Image, cfg config.Config) detect.Locator{
		"ncc": func(tmpl image.Image, cfg config.Config) detect.Locator { return detect.NewNCCLocator(tmpl, cfg) },
	}
	biteDetectors = map[string]func() detect.BiteDetector{
		"motion": func() detect.BiteDetector { return detect.NewMotionBiteDetector(slog.New(slog.DiscardHandler)) },
	}
)

func main() {
	configPath := flag.String("config", "", "config file (default: built-in defaults)")
	locatorName := flag.String("locator", "ncc", "locator: "+strings.Join(slices.Sorted(maps.Keys(locators)), ", "))
	biteName := flag.String("bite", "motion", "bite detector: "+strings.Join(slices.Sorted(maps.Keys(biteDetectors)), ", "))
	asJSON := flag.Bool("json", false, "print the counts as JSON, without timings")
	label := flag.String("label", "", "write labels.json for this recorded session and exit")
	cast := flag.String("cast", "mouse:x1", "with -label: input that casts, for example mouse:x1 or key:F3")
	reel := flag.String("reel", "mouse:right", "with -label: input that reels")
	reaction := flag.Int64("reaction", 400, "with -label: assumed ms from the bite to the reel click")
	box := flag.Int("box", 40, "with -label: side in pixels of the bobber box around the reel click")
	flag.Parse()
	var err error
	if *label != "" {
		err = writeLabels(*label, dataset.AutoLabelOptions{Cast: *cast, Reel: *reel, ReactionMS: *reaction, BoxPx: *box}, os.Stdout)
	} else {
		err = run(*configPath, *locatorName, *biteName, *asJSON, flag.Args(), os.Stdout)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "detecteval:", err)
		os.Exit(1)
	}
}

func writeLabels(dir string, opts dataset.AutoLabelOptions, out io.Writer) error {
	labels, skipped, err := dataset.AutoLabel(dir, opts)
	if err != nil {
		return err
	}
	if err := dataset.WriteLabels(dir, labels); err != nil {
		return err
	}
	fmt.Fprintf(out, "Labelled %d casts and %d spans with no bobber.\n", len(labels.Casts), len(labels.Empty))
	for _, reason := range skipped {
		fmt.Fprintln(out, "Skipped:", reason)
	}
	return nil
}

type row struct {
	Name   string `json:"name"`
	Counts Counts `json:"counts"`
}

func run(configPath, locatorName, biteName string, asJSON bool, dirs []string, out io.Writer) error {
	newLocator, ok := locators[locatorName]
	if !ok {
		return fmt.Errorf("unknown locator %q", locatorName)
	}
	newBite, ok := biteDetectors[biteName]
	if !ok {
		return fmt.Errorf("unknown bite detector %q", biteName)
	}
	if len(dirs) == 0 {
		return fmt.Errorf("no datasets given")
	}
	cfg := config.DefaultConfig()
	if configPath != "" {
		var err error
		if cfg, err = config.Load(configPath); err != nil {
			return err
		}
	}
	tmpl, err := assets.FishingTargetImage()
	if err != nil {
		return err
	}

	eval := &evaluator{locator: newLocator(tmpl, *cfg), bite: newBite(), roiSize: cfg.ROISizePx}
	var rows []row
	var total Counts
	for _, dir := range dirs {
		d, err := dataset.Load(dir)
		if err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		counts, err := eval.run(d)
		if err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		rows = append(rows, row{d.Name, counts})
		total.add(counts)
	}
	rows = append(rows, row{"total", total})
	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}
	return printTable(out, rows, eval.searches)
}

func printTable(out io.Writer, rows []row, searches []time.Duration) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	metrics := []struct {
		name, target string
		value        func(Counts) string
	}{
		{"Casts", "", func(c Counts) string { return fmt.Sprint(c.Casts) }},
		{"Bobber found per cast", ">= 98%", func(c Counts) string { return percent(c.Locks, c.Casts) }},
		{"False locks on empty frames", "<= 1%", func(c Counts) string { return percent(c.FalseLocks, c.EmptyFrames) }},
		{"Click point inside the bobber", ">= 99%", func(c Counts) string { return percent(c.LocksOnBobber, c.Locks) }},
		{"Bites caught", ">= 98%", func(c Counts) string { return percent(c.BitesCaught, c.BiteCasts) }},
		{"False bites per cast", "<= 2%", func(c Counts) string { return percent(c.FalseBites, c.Casts) }},
		{"Bite latency p95", "<= 300 ms", latency},
	}
	header := []string{"Metric", "Target"}
	for _, r := range rows {
		header = append(header, r.Name)
	}
	fmt.Fprintln(w, strings.Join(header, "\t"))
	for _, m := range metrics {
		cells := []string{m.name, m.target}
		for _, r := range rows {
			cells = append(cells, m.value(r.Counts))
		}
		fmt.Fprintln(w, strings.Join(cells, "\t"))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(out, "\nSearch time p95: %v (target <= 100ms, varies per run)\n", percentile95(searches).Round(time.Millisecond))
	return err
}

// percent shows n/a when nothing was measured, so an empty result never looks
// like it met its target.
func percent(n, of int) string {
	if of == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.1f%%", float64(n)/float64(of)*100)
}

func latency(c Counts) string {
	if len(c.BiteLatenciesMS) == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d ms", percentile95(c.BiteLatenciesMS))
}
