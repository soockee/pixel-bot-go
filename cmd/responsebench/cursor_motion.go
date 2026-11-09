package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"github.com/soocke/pixel-bot-go/internal/cursormotion"
	"github.com/soocke/pixel-bot-go/internal/dataset"
	"github.com/soocke/pixel-bot-go/internal/playtrace"
)

type cursorPoint struct {
	us int64
	p  image.Point
}

type cursorBurst struct {
	epoch  int
	bounds image.Rectangle
	points []cursorPoint
}

type cursorExample struct {
	sample        cursormotion.Sample
	start, target image.Point
	bounds        image.Rectangle
	reelUS        int64
	human         motionMetrics
	points        []cursormotion.Point
}

type motionMetrics struct {
	DurationMS  float64 `json:"duration_ms"`
	TravelRatio float64 `json:"travel_ratio"`
	Lateral     float64 `json:"peak_lateral_fraction"`
}

func motionPosition(p *playtrace.Position) (image.Point, image.Rectangle, bool) {
	if p == nil {
		return image.Point{}, image.Rectangle{}, false
	}
	point := image.Pt(int(p.X), int(p.Y))
	bounds := image.Rectangle{Max: image.Pt(int(p.Width), int(p.Height))}
	return point, bounds, point.In(bounds)
}

func cursorDistance(a, b image.Point) float64 {
	return math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y))
}

func measureMotion(start, target image.Point, steps []cursormotion.Step) motionMetrics {
	dx, dy := float64(target.X-start.X), float64(target.Y-start.Y)
	d2 := dx*dx + dy*dy
	var m motionMetrics
	last := start
	for _, step := range steps {
		m.TravelRatio += cursorDistance(last, step.Point)
		x, y := float64(step.Point.X-start.X), float64(step.Point.Y-start.Y)
		m.Lateral = max(m.Lateral, math.Abs(dx*y-dy*x)/d2)
		m.DurationMS = float64(step.After) / float64(time.Millisecond)
		last = step.Point
	}
	m.TravelRatio /= math.Sqrt(d2)
	return m
}

// extractApproach finds the last substantial reach followed only by local
// adjustments at the reviewed click. Distances are client pixels.
func extractApproach(bursts []cursorBurst, epoch int, bounds image.Rectangle, target image.Point, castUS, reelUS int64) (cursorExample, bool) {
	for i := len(bursts) - 1; i >= 0; i-- {
		b := bursts[i]
		if b.epoch != epoch || b.bounds != bounds || len(b.points) < 3 {
			continue
		}
		first, last := b.points[0], b.points[len(b.points)-1]
		if first.us < castUS || last.us >= reelUS || last.us-first.us < 70_000 || last.us-first.us > 1_500_000 {
			continue
		}
		distance := cursorDistance(first.p, target)
		if distance < 25 || cursorDistance(first.p, last.p) < 20 || cursorDistance(last.p, target) > max(18, min(28, distance*.18)) {
			continue
		}
		var settle *cursormotion.Settle
		local := true
		for _, adjustment := range bursts[i+1:] {
			if adjustment.epoch != epoch || len(adjustment.points) == 0 || adjustment.points[0].us >= reelUS {
				break
			}
			for _, p := range adjustment.points {
				if p.us < reelUS && (adjustment.bounds != bounds || cursorDistance(p.p, target) > 32) {
					local = false
				}
			}
			a, z := adjustment.points[0], adjustment.points[len(adjustment.points)-1]
			gap, duration := a.us-last.us, z.us-a.us
			if settle == nil && cursorDistance(last.p, target) >= 2 &&
				z.us < reelUS && cursorDistance(z.p, target) <= 3 &&
				gap >= 50_000 && gap <= 1_500_000 && duration >= 10_000 && duration <= 250_000 {
				settle = &cursormotion.Settle{PauseUS: gap, DurationUS: duration}
			}
		}
		if !local {
			continue
		}
		s := cursormotion.Sample{Distance: distance, Settle: settle}
		dx, dy := float64(target.X-first.p.X), float64(target.Y-first.p.Y)
		d2 := dx*dx + dy*dy
		var actual []cursormotion.Step
		points := make([]cursormotion.Point, 0, len(b.points))
		for n, p := range b.points {
			dt := p.us - first.us
			actual = append(actual, cursormotion.Step{After: time.Duration(dt) * time.Microsecond, Point: p.p})
			x, y := float64(p.p.X-first.p.X), float64(p.p.Y-first.p.Y)
			point := cursormotion.Point{US: dt, Along: (dx*x + dy*y) / d2, Across: (dx*y - dy*x) / d2}
			points = append(points, point)
			if n%(1+(len(b.points)-1)/127) == 0 || n == len(b.points)-1 {
				s.Points = append(s.Points, point)
			}
		}
		if s.Valid() {
			return cursorExample{s, first.p, target, bounds, reelUS, measureMotion(first.p, target, actual), points}, true
		}
	}
	return cursorExample{}, false
}

func readCursorSession(dir, cast, reel string) (cursormotion.Session, []cursorExample, error) {
	timing, _, err := readTimingSession(dir, cast, reel)
	if err != nil {
		return cursormotion.Session{}, nil, err
	}
	session := cursormotion.Session{Name: timing.Name, TraceSHA256: timing.TraceSHA256, ReviewSHA256: timing.ReviewSHA256}
	reviewed := map[int64]int64{}
	if timing.Status == "complete" {
		for _, c := range timing.Casts {
			if c.Review != nil && c.Review.ReelUS != nil {
				reviewed[*c.Review.ReelUS] = c.Review.CastUS
			}
		}
	}
	if len(reviewed) == 0 {
		return session, nil, nil
	}
	binding, err := parseBinding(timing.ReelBinding)
	if err != nil {
		return session, nil, err
	}
	file, err := os.Open(dataset.InputPath(dir))
	if err != nil {
		return session, nil, err
	}
	defer file.Close()
	dec := json.NewDecoder(file)
	var first json.RawMessage
	if err := dec.Decode(&first); err != nil {
		return session, nil, err
	}
	header, err := playtrace.DecodeHeader(first)
	if err != nil {
		return session, nil, err
	}
	var examples []cursorExample
	var bursts []cursorBurst
	var burst cursorBurst
	epoch, active := 0, header.Schema < 3
	held := map[string]bool{}
	flush := func() {
		if len(burst.points) != 0 {
			bursts = append(bursts, burst)
			burst = cursorBurst{}
		}
	}
	err = playtrace.ReadEvents(dec, func(e playtrace.Event) error {
		switch e.Type {
		case "segment_start", "segment_end", "mouse_gap":
			flush()
			bursts = nil
			clear(held)
			epoch++
			if e.Type != "mouse_gap" {
				active = e.Type == "segment_start"
			}
		case "button_down":
			flush()
			held[e.Button] = true
		case "button_up":
			delete(held, e.Button)
		case "key_down", "wheel", "wheel_horizontal":
			flush()
		case "move":
			p, area, ok := motionPosition(e.Position)
			if !active || !ok || len(held) != 0 {
				flush()
				return nil
			}
			if n := len(burst.points); n > 0 {
				last := burst.points[n-1]
				if area != burst.bounds || e.ElapsedUS-last.us > 150_000 {
					flush()
				} else if p == last.p || e.ElapsedUS == last.us {
					return nil
				}
			}
			burst.epoch, burst.bounds = epoch, area
			burst.points = append(burst.points, cursorPoint{e.ElapsedUS, p})
		case "click":
			press, matches := binding.match(e)
			cast, wanted := reviewed[press]
			if !active || !matches || !wanted {
				return nil
			}
			flush()
			p, area, ok := motionPosition(&e.Gesture.Start)
			if !ok {
				return nil
			}
			if example, ok := extractApproach(bursts, epoch, area, p, cast, press); ok {
				examples = append(examples, example)
			}
		}
		return nil
	})
	session.Paths = len(examples)
	return session, examples, err
}

type cursorErrors struct {
	DurationMS  float64 `json:"mean_absolute_duration_error_ms"`
	TravelRatio float64 `json:"mean_absolute_travel_ratio_error"`
	Lateral     float64 `json:"mean_absolute_lateral_fraction_error"`
}

func (e *cursorErrors) add(actual, expected motionMetrics, weight float64) {
	e.DurationMS += math.Abs(actual.DurationMS-expected.DurationMS) * weight
	e.TravelRatio += math.Abs(actual.TravelRatio-expected.TravelRatio) * weight
	e.Lateral += math.Abs(actual.Lateral-expected.Lateral) * weight
}

func runCursorFit(dirs []string, cast, reel, train, modelPath string, out io.Writer) error {
	if len(dirs) == 0 || train == "" {
		return errors.New("-cursor-fit needs recording directories and -cursor-train")
	}
	training := map[string]bool{}
	for name := range strings.SplitSeq(train, ",") {
		training[strings.TrimSpace(name)] = true
	}
	model := cursormotion.Model{Version: cursormotion.Version}
	seen, hashes := map[string]bool{}, map[string]bool{}
	var holdout [][]cursorExample
	var heldOutPaths int
	for _, dir := range dirs {
		session, examples, err := readCursorSession(dir, cast, reel)
		if err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		if seen[session.Name] || hashes[session.TraceSHA256] {
			return fmt.Errorf("duplicate recording name or trace: %s", dir)
		}
		seen[session.Name], hashes[session.TraceSHA256] = true, true
		if training[session.Name] {
			model.Training = append(model.Training, session)
			for _, e := range examples {
				model.Samples = append(model.Samples, e.sample)
			}
		} else {
			model.Holdout = append(model.Holdout, session)
			holdout = append(holdout, examples)
			heldOutPaths += len(examples)
		}
	}
	for name := range training {
		if !seen[name] {
			return fmt.Errorf("training recording %q was not supplied", name)
		}
	}
	if len(model.Samples) == 0 || heldOutPaths == 0 {
		return errors.New("need reviewed cursor approaches in both training and held-out recordings")
	}
	const repeats = 12
	var report struct {
		Training        []cursormotion.Session `json:"training"`
		Holdout         []cursormotion.Session `json:"holdout"`
		TrialsPerReach  int                    `json:"trials_per_reach"`
		Direct          cursorErrors           `json:"direct"`
		Fitted          cursorErrors           `json:"fitted"`
		Default         cursorErrors           `json:"default"`
		HumanSettles    int                    `json:"human_settles"`
		FittedSettles   int                    `json:"fitted_settles"`
		HumanApproaches []motionMetrics        `json:"human_approaches"`
		ReachAudits     []sessionReachAudit    `json:"reach_audits"`
	}
	report.Training, report.Holdout, report.TrialsPerReach = model.Training, model.Holdout, repeats
	for i, examples := range holdout {
		audit := sessionReachAudit{Session: model.Holdout[i].Name, DirectInstantMoves: len(examples)}
		var human reachHistory
		for _, example := range examples {
			human.observe(example.points)
			report.HumanApproaches = append(report.HumanApproaches, example.human)
			if example.sample.Settle != nil {
				report.HumanSettles++
			}
			report.Direct.add(motionMetrics{TravelRatio: 1}, example.human, 1/float64(heldOutPaths))
			for seed := uint64(1); seed <= repeats; seed++ {
				steps := model.Planner(seed)(example.start, example.target, example.bounds, false)
				report.Fitted.add(measureMotion(example.start, example.target, steps), example.human, 1/float64(heldOutPaths*repeats))
				fallback := cursormotion.DefaultPath(example.start, example.target, example.bounds, false)
				report.Default.add(measureMotion(example.start, example.target, fallback), example.human, 1/float64(heldOutPaths*repeats))
				withSettle := model.Planner(seed)(example.start, example.target, example.bounds, true)
				if withSettle[len(withSettle)-1].After > steps[len(steps)-1].After {
					report.FittedSettles++
				}
			}
		}
		audit.Human = human.result()
		for seed := uint64(1); seed <= repeats; seed++ {
			audit.Fitted = append(audit.Fitted, auditFittedReaches(examples, model.Planner(seed)))
			audit.Default = append(audit.Default, auditFittedReaches(examples, cursormotion.DefaultPath))
		}
		report.ReachAudits = append(report.ReachAudits, audit)
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil || modelPath == "" {
		return err
	}
	return writeProfile(modelPath, model)
}
