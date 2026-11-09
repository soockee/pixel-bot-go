// Command responsebench compares pacing and retry policies on seeded synthetic
// workloads with a virtual clock. It is standalone: it uses neither the desktop
// application, its state machine, nor any operating-system input path.
//
// Usage: responsebench [-out dir] [-split eval|dev] [-config file.json]
//
//	[-trace-feedback] [-seeds n,n,...]
//
// The default run evaluates the frozen controllers on the eval split. The dev
// split ranks every candidate, which is how the frozen settings were chosen.
// Trace feedback compares three profiles on either split.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/soocke/pixel-bot-go/internal/inputtiming"
	"github.com/soocke/pixel-bot-go/internal/playtrace"
	"github.com/soocke/pixel-bot-go/internal/traceobserver"
)

// Manifest records what produced a result directory. It is left out of
// CHECKSUMS.txt so runs on different machines can be compared directly.
type Manifest struct {
	Args         []string `json:"args"`
	Split        string   `json:"split"`
	Revision     string   `json:"revision"`
	DirtyFiles   []string `json:"dirty_files"`
	GoVersion    string   `json:"go_version"`
	GOOS         string   `json:"goos"`
	GOARCH       string   `json:"goarch"`
	ConfigSHA256 string   `json:"config_sha256"`
}

func main() {
	out := flag.String("out", "responsebench-results/eval", "new result directory; must not exist")
	split := flag.String("split", "eval", "eval runs the frozen pair; dev ranks all candidates")
	configPath := flag.String("config", "", "experiment definition (default: embedded config.json)")
	feedback := flag.Bool("trace-feedback", false, "compare the frozen pair and the feedback-enabled profile")
	seeds := flag.String("seeds", "", "comma-separated subset of seeds from the selected split")
	compareTraces := flag.Bool("compare", false, "compare JSONL traces supplied as positional paths")
	asJSON := flag.Bool("json", false, "emit JSON instead of Markdown in comparison mode")
	manualCast := flag.String("manual-cast", "", "optional manual cast binding, for example mouse:x1")
	manualReel := flag.String("manual-reel", "", "optional manual reel binding, for example mouse:right")
	matchWindow := flag.Duration("match-window", 1500*time.Millisecond, "maximum aligned bite-to-response delay")
	inputTiming := flag.Bool("input-timing", false, "report candidate and reviewed timing from recording directories as JSON")
	timingTrain := flag.String("timing-train", "", "comma-separated training directory names; remaining sessions are held out")
	timingModel := flag.String("timing-model-out", "", "new file for a frozen dry-run timing model")
	timingCalibration := flag.String("timing-calibration", "", "JSON comparison of fixed simulator runs used to measure detector lag")
	cursorFit := flag.Bool("cursor-fit", false, "fit and compare cursor paths against held-out recording directories")
	cursorTrain := flag.String("cursor-train", "", "comma-separated training directory names; remaining sessions are held out")
	cursorModel := flag.String("cursor-model-out", "", "new file for a frozen cursor profile")
	cursorAudit := flag.Bool("cursor-audit", false, "summarize pointer reaches of dry-run traces (files) and reviewed recordings (directories) as JSON")
	flag.Parse()
	if *cursorAudit {
		if err := runCursorAudit(flag.Args(), *manualCast, *manualReel, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "responsebench:", err)
			os.Exit(1)
		}
		return
	}
	if *cursorFit {
		if err := runCursorFit(flag.Args(), *manualCast, *manualReel, *cursorTrain, *cursorModel, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "responsebench:", err)
			os.Exit(1)
		}
		return
	}
	if *inputTiming {
		var calibration *inputtiming.Calibration
		if *timingCalibration != "" {
			var err error
			calibration, err = readTimingCalibration(*timingCalibration)
			if err != nil {
				fmt.Fprintln(os.Stderr, "responsebench: timing calibration:", err)
				os.Exit(1)
			}
		}
		if err := runInputTiming(flag.Args(), *manualCast, *manualReel, *timingTrain, *timingModel, calibration, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "responsebench:", err)
			os.Exit(1)
		}
		return
	}
	if *compareTraces {
		if err := runTraceComparison(flag.Args(), *manualCast, *manualReel, *matchWindow, *asJSON, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "responsebench:", err)
			os.Exit(1)
		}
		return
	}
	if err := runExperiment(*out, *split, *configPath, os.Stdout, *feedback, *seeds); err != nil {
		fmt.Fprintln(os.Stderr, "responsebench:", err)
		os.Exit(1)
	}
}

func run(out, split, configPath string, stdout io.Writer) error {
	return runExperiment(out, split, configPath, stdout, false, "")
}

func runExperiment(out, split, configPath string, stdout io.Writer, feedback bool, seedList string) error {
	raw := defaultConfig
	if configPath != "" {
		var err error
		if raw, err = os.ReadFile(configPath); err != nil {
			return err
		}
	}
	cfg, err := parseConfig(raw)
	if err != nil {
		return err
	}
	if split != "eval" && split != "dev" {
		return fmt.Errorf("unknown split %q", split)
	}
	if cfg.selectedSeeds, err = parseSeeds(seedList, cfg.Splits[split]); err != nil {
		return err
	}
	if feedback && cfg.TraceFeedback == nil {
		return fmt.Errorf("config: -trace-feedback requires trace_feedback settings")
	}
	if !feedback {
		cfg.TraceFeedback = nil
	}

	controllers := cfg.Candidates
	if split == "eval" || feedback {
		controllers = nil
		for _, name := range cfg.Frozen {
			i := slices.IndexFunc(cfg.Candidates, func(c ControllerConfig) bool { return c.Name == name })
			controllers = append(controllers, cfg.Candidates[i])
		}
	}
	if feedback {
		variant := controllers[1]
		if variant.Kind != kindBackoff || controllers[0].Name == variant.Name ||
			strings.HasSuffix(controllers[0].Name, feedbackSuffix) || strings.HasSuffix(variant.Name, feedbackSuffix) {
			return fmt.Errorf("config: trace feedback needs distinct frozen controllers with a backoff treatment and no %q suffix", feedbackSuffix)
		}
		variant.Name += feedbackSuffix
		controllers = append(controllers, variant)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	// Mkdir fails on an existing directory, so saved runs are never overwritten.
	if err := os.Mkdir(out, 0o755); err != nil {
		return fmt.Errorf("result directory must be new: %w", err)
	}

	files := []string{"config.json", "scenarios.jsonl", "metrics.jsonl", "summary.json", "summary.md"}
	if feedback && cfg.Shadow != nil {
		cfg.fittedShadow, err = fitShadow(cfg)
		if err != nil {
			return err
		}
		data, err := json.MarshalIndent(cfg.fittedShadow, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, "shadow-model.json"), append(data, '\n'), 0o644); err != nil {
			return err
		}
		files = append(files, "shadow-model.json")
	}
	if err := os.WriteFile(filepath.Join(out, "config.json"), raw, 0o644); err != nil {
		return err
	}
	// The dev grid is about 25 times the eval size, so only eval keeps raw traces.
	traces := io.Writer(nil)
	if split == "eval" || feedback {
		f, err := os.Create(filepath.Join(out, "traces.jsonl"))
		if err != nil {
			return err
		}
		defer f.Close()
		w := bufio.NewWriter(f)
		defer w.Flush()
		traces = w
		files = append(files, "traces.jsonl")
		cfg.commonTrace, err = playtrace.Create(filepath.Join(out, "playtrace.jsonl"),
			playtrace.Header{Source: playtrace.Benchmark, Clock: playtrace.Virtual})
		if err != nil {
			return err
		}
		defer cfg.commonTrace.Close(playtrace.End{Status: "incomplete"})
		files = append(files, "playtrace.jsonl")
	}
	scenarios, err := os.Create(filepath.Join(out, "scenarios.jsonl"))
	if err != nil {
		return err
	}
	defer scenarios.Close()

	metrics, err := runAll(cfg, split, controllers, scenarios, traces)
	if err != nil {
		return err
	}
	if cfg.commonTrace != nil {
		if err := cfg.commonTrace.Close(playtrace.End{Status: "complete"}); err != nil {
			return err
		}
	}
	if w, ok := traces.(*bufio.Writer); ok {
		if err := w.Flush(); err != nil {
			return err
		}
	}
	if err := writeJSONL(filepath.Join(out, "metrics.jsonl"), metrics); err != nil {
		return err
	}

	var report string
	var summary any
	if feedback {
		pairs := [][2]string{
			{controllers[0].Name, controllers[1].Name},
			{controllers[0].Name, controllers[2].Name},
			{controllers[1].Name, controllers[2].Name},
		}
		var comparisons []traceComparison
		report = traceReport(metrics)
		if cfg.fittedShadow != nil {
			references, err := evaluateReference(cfg, split)
			if err != nil {
				return err
			}
			if err := writeJSONL(filepath.Join(out, "shadow-reference.jsonl"), references); err != nil {
				return err
			}
			files = append(files, "shadow-reference.jsonl")
			report += shadowReport(cfg.fittedShadow, metrics, references)
		}
		for _, pair := range pairs {
			rows, err := compare(cfg, metrics, pair[0], pair[1])
			if err != nil {
				return err
			}
			comparisons = append(comparisons, traceComparison{pair[0], pair[1], rows})
			report += "\n" + comparisonReport(split, pair[0], pair[1], rows)
		}
		summary = comparisons
	} else if split == "dev" {
		sel, err := selectControllers(cfg, metrics)
		if err != nil {
			return err
		}
		summary, report = sel, selectionReport(sel)
	} else {
		rows, err := compare(cfg, metrics, cfg.Frozen[0], cfg.Frozen[1])
		if err != nil {
			return err
		}
		summary, report = rows, comparisonReport(split, cfg.Frozen[0], cfg.Frozen[1], rows)
	}
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "summary.json"), append(data, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "summary.md"), []byte(report), 0o644); err != nil {
		return err
	}

	revision, dirty := gitState()
	sum := sha256.Sum256(raw)
	manifest, err := json.MarshalIndent(Manifest{
		Args: os.Args, Split: split, Revision: revision, DirtyFiles: dirty,
		GoVersion: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		ConfigSHA256: hex.EncodeToString(sum[:]),
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "manifest.json"), append(manifest, '\n'), 0o644); err != nil {
		return err
	}
	if err := writeChecksums(out, files); err != nil {
		return err
	}
	fmt.Fprintln(stdout, report)
	fmt.Fprintln(stdout, "results saved to", out)
	if len(dirty) > 0 {
		fmt.Fprintln(os.Stderr, "responsebench: warning: uncommitted changes; this run is not tied to a clean revision")
	}
	return nil
}

func parseSeeds(list string, split Split) ([]uint64, error) {
	if list == "" {
		return nil, nil
	}
	var seeds []uint64
	for item := range strings.SplitSeq(list, ",") {
		seed, err := strconv.ParseUint(strings.TrimSpace(item), 10, 64)
		if err != nil || seed < split.BaseSeed || seed-split.BaseSeed >= uint64(split.Runs) {
			return nil, fmt.Errorf("seed %q is outside the selected split", item)
		}
		if slices.Contains(seeds, seed) {
			return nil, fmt.Errorf("duplicate seed %d", seed)
		}
		seeds = append(seeds, seed)
	}
	slices.Sort(seeds)
	return seeds, nil
}

func benchmarkSeeds(cfg Config, split string) []uint64 {
	if cfg.selectedSeeds != nil {
		return cfg.selectedSeeds
	}
	s := cfg.Splits[split]
	seeds := make([]uint64, s.Runs)
	for i := range seeds {
		seeds[i] = s.BaseSeed + uint64(i)
	}
	return seeds
}

// runAll runs every controller on every scenario, in a fixed order.
func runAll(cfg Config, split string, controllers []ControllerConfig, scenarios, traces io.Writer) ([]Metrics, error) {
	sw := bufio.NewWriter(scenarios)
	scenarioEnc := json.NewEncoder(sw)
	var traceEnc *json.Encoder
	if traces != nil {
		traceEnc = json.NewEncoder(traces)
	}
	var metrics []Metrics
	seeds := benchmarkSeeds(cfg, split)
	for _, fam := range cfg.Families {
		for _, seed := range seeds {
			sc := generate(cfg, fam, split, seed)
			if err := scenarioEnc.Encode(sc); err != nil {
				return nil, err
			}
			for _, cc := range controllers {
				var trace []Record
				var score *traceScore
				var observer *traceobserver.Observer
				var observers []func(Record)
				if cfg.fittedShadow != nil {
					observer = newShadowObserver(cfg.fittedShadow, sc)
					observers = append(observers, observer.Observe)
				}
				var err error
				if cfg.TraceFeedback == nil {
					trace, err = simulate(sc, newController(cc, sc.Seed), cc.Name)
				} else {
					trace, score, err = simulateWithFeedback(sc, cc, *cfg.TraceFeedback, observers...)
				}
				if err != nil {
					return nil, err
				}
				m, err := computeMetrics(sc, cc.Name, trace)
				if err != nil {
					return nil, err
				}
				m.TraceScore = score
				if observer != nil {
					result := observer.Finish()
					m.Shadow = &result
				}
				metrics = append(metrics, m)
				if traceEnc == nil {
					continue
				}
				for _, r := range trace {
					if cfg.commonTrace != nil {
						if err := cfg.commonTrace.Write(benchmarkEvent(r)); err != nil {
							return nil, err
						}
					}
					if err := traceEnc.Encode(r); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	return metrics, sw.Flush()
}

func writeJSONL[T any](path string, rows []T) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return w.Flush()
}

// writeChecksums writes CHECKSUMS.txt last, in sha256sum format. A result
// directory without it did not finish.
func writeChecksums(dir string, files []string) error {
	slices.Sort(files)
	var b strings.Builder
	for _, name := range files {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%x  %s\n", h.Sum(nil), name)
	}
	return os.WriteFile(filepath.Join(dir, "CHECKSUMS.txt"), []byte(b.String()), 0o644)
}

// gitState returns the commit and uncommitted paths. Outside a repository the
// revision is "unknown", which the manifest records instead of failing the run.
func gitState() (string, []string) {
	rev, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown", nil
	}
	status, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		return strings.TrimSpace(string(rev)), []string{"status unavailable"}
	}
	dirty := []string{}
	for line := range strings.Lines(string(status)) {
		dirty = append(dirty, strings.TrimRight(line, "\r\n"))
	}
	return strings.TrimSpace(string(rev)), dirty
}
