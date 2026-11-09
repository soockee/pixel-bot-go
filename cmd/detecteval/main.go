// Command detecteval compares bobber locators and bite detectors on recorded
// datasets. It prints the metrics from docs/ARCHITECTURE_REVIEW.md with their
// targets, one column per technique.
//
// Usage: detecteval [-config file] [-locator names] [-bite names] [-json] dataset-dir...
//
//	detecteval -label dataset-dir [-cast binding] [-reel binding] [-reaction ms] [-box px]
//	detecteval -export new-dir [-config file] dataset-dir...
//
// -locator and -bite take comma-separated names or "all". -label writes
// labels.json from a session's cast and reel input. -export saves each cast's
// watched area as small images for training.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/detect"
	"github.com/soocke/pixel-bot-go/internal/dataset"
)

func main() {
	configPath := flag.String("config", "", "config file (default: built-in defaults)")
	locatorNames := flag.String("locator", "all", "locators to compare: all, or some of "+strings.Join(detect.LocatorNames(), ","))
	biteNames := flag.String("bite", "all", "bite detectors to compare: all, or some of "+strings.Join(detect.BiteDetectorNames(), ","))
	asJSON := flag.Bool("json", false, "print the counts as JSON, without timings")
	label := flag.String("label", "", "write labels.json for this recorded session and exit")
	labelBot := flag.String("label-bot", "", "write labels.json for this recording of a bot run, from the outcomes in its bot.jsonl, and exit")
	cast := flag.String("cast", "mouse:x1", "with -label: input that casts, for example mouse:x1 or key:F3")
	reel := flag.String("reel", "mouse:right", "with -label: input that reels")
	reaction := flag.Int64("reaction", 1200, "with -label: assumed ms from the bite to the reel click")
	box := flag.Int("box", 40, "with -label: side in pixels of the bobber box around the reel click")
	exportDir := flag.String("export", "", "save each cast's watched area into this new directory and exit")
	flag.Parse()
	var err error
	switch {
	case *label != "":
		err = writeLabels(*label, dataset.AutoLabelOptions{Cast: *cast, Reel: *reel, ReactionMS: *reaction, BoxPx: *box}, os.Stdout)
	case *labelBot != "":
		err = writeBotLabels(*labelBot, *box, os.Stdout)
	case *exportDir != "":
		var cfg *config.Config
		if cfg, err = loadConfig(*configPath); err == nil {
			err = export(*exportDir, flag.Args(), cfg.ROISizePx, os.Stdout)
		}
	default:
		err = run(*configPath, *locatorNames, *biteNames, *asJSON, flag.Args(), os.Stdout)
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

func loadConfig(path string) (*config.Config, error) {
	if path == "" {
		return config.DefaultConfig(), nil
	}
	return config.Load(path)
}

// pickNames expands "all" and checks the names against those registered.
func pickNames(arg string, registered []string) ([]string, error) {
	if arg == "all" {
		return registered, nil
	}
	names := strings.Split(arg, ",")
	for _, name := range names {
		if !slices.Contains(registered, name) {
			return nil, fmt.Errorf("unknown technique %q, have %s", name, strings.Join(registered, ", "))
		}
	}
	return names, nil
}

// Report holds the results of a comparison over all datasets.
type Report struct {
	Locators map[string]LocatorCounts `json:"locators"`
	Bites    map[string]BiteCounts    `json:"bites"`
}

func run(configPath, locatorArg, biteArg string, asJSON bool, dirs []string, out io.Writer) error {
	locators, err := pickNames(locatorArg, detect.LocatorNames())
	if err != nil {
		return err
	}
	bites, err := pickNames(biteArg, detect.BiteDetectorNames())
	if err != nil {
		return err
	}
	if len(dirs) == 0 {
		return fmt.Errorf("no datasets given")
	}
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	report := Report{Locators: map[string]LocatorCounts{}, Bites: map[string]BiteCounts{}}
	searches := map[string][]time.Duration{}
	for _, dir := range dirs {
		d, err := dataset.Load(dir)
		if err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		for _, name := range locators {
			locator, err := detect.NewLocator(name, *cfg)
			if err != nil {
				return err
			}
			counts := report.Locators[name]
			times, err := evaluateLocator(locator, d, &counts)
			if err != nil {
				return fmt.Errorf("%s: %w", dir, err)
			}
			report.Locators[name] = counts
			searches[name] = append(searches[name], times...)
		}
		watches, err := watchAreas(d, cfg.ROISizePx)
		if err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		for _, name := range bites {
			bite, err := detect.NewBiteDetector(name, slog.New(slog.DiscardHandler))
			if err != nil {
				return err
			}
			counts := report.Bites[name]
			if err := evaluateBite(bite, d, watches, &counts); err != nil {
				return fmt.Errorf("%s: %w", dir, err)
			}
			report.Bites[name] = counts
		}
	}
	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	return printReport(out, report, locators, bites, searches)
}

func printReport(out io.Writer, r Report, locators, bites []string, searches map[string][]time.Duration) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	table := func(title string, names []string, rows [][2]string, cell func(row int, name string) string) {
		fmt.Fprintln(w, strings.Join(append([]string{title, "Target"}, names...), "\t"))
		for i, row := range rows {
			cells := []string{row[0], row[1]}
			for _, name := range names {
				cells = append(cells, cell(i, name))
			}
			fmt.Fprintln(w, strings.Join(cells, "\t"))
		}
		fmt.Fprintln(w)
	}
	table("Locator", locators, [][2]string{
		{"Casts", ""}, {"Bobber found per cast", ">= 98%"}, {"False locks on empty frames", "<= 1%"},
		{"Click point inside the bobber", ">= 99%"}, {"Search time p95, varies per run", "<= 100ms"},
	}, func(row int, name string) string {
		c := r.Locators[name]
		switch row {
		case 0:
			return fmt.Sprint(c.Casts)
		case 1:
			return percent(c.Locks, c.Casts)
		case 2:
			return percent(c.FalseLocks, c.EmptyFrames)
		case 3:
			return percent(c.LocksOnBobber, c.Locks)
		}
		return percentile95(searches[name]).Round(100 * time.Microsecond).String()
	})
	table("Bite detector", bites, [][2]string{
		{"Casts", ""}, {"Bites caught", ">= 98%"}, {"False bites per cast", "<= 2%"}, {"Bite latency p95", "<= 300 ms"},
	}, func(row int, name string) string {
		c := r.Bites[name]
		switch row {
		case 0:
			return fmt.Sprint(c.Casts)
		case 1:
			return percent(c.BitesCaught, c.BiteCasts)
		case 2:
			return percent(c.FalseBites, c.Casts)
		}
		return latency(c)
	})
	return w.Flush()
}

// percent shows n/a when nothing was measured, so an empty result never looks
// like it met its target.
func percent(n, of int) string {
	if of == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.1f%%", float64(n)/float64(of)*100)
}

func latency(c BiteCounts) string {
	if len(c.BiteLatenciesMS) == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d ms", percentile95(c.BiteLatenciesMS))
}

// writeBotLabels labels a recording of a bot run from the outcomes in its bot.jsonl.
func writeBotLabels(dir string, box int, out io.Writer) error {
	labels, skipped, err := dataset.BotLabel(dir, box)
	if err != nil {
		return err
	}
	if err := dataset.WriteLabels(dir, labels); err != nil {
		return err
	}
	bites := 0
	for _, c := range labels.Casts {
		if c.BiteMS != nil {
			bites++
		}
	}
	fmt.Fprintf(out, "Labelled %d casts from the bot's outcomes: %d with a bite, %d with none.\n", len(labels.Casts), bites, len(labels.Casts)-bites)
	for _, reason := range skipped {
		fmt.Fprintln(out, "Skipped:", reason)
	}
	return nil
}
