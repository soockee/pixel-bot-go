package main

import (
	"cmp"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/soocke/pixel-bot-go/internal/dataset"
	"github.com/soocke/pixel-bot-go/internal/inputtiming"
	"github.com/soocke/pixel-bot-go/internal/playtrace"
)

const reviewFile = "timing-review.json"

type cueReview struct {
	EarliestUS int64  `json:"earliest_us"`
	LatestUS   int64  `json:"latest_us"`
	Source     string `json:"source"` // visual, audio or simulator
}

type castReview struct {
	CastUS   int64      `json:"cast_us"`
	Outcome  string     `json:"outcome"`
	ReelUS   *int64     `json:"reel_us,omitempty"`
	Cue      *cueReview `json:"cue,omitempty"`
	Evidence string     `json:"evidence,omitempty"`
}

type timingReview struct {
	CastBinding string       `json:"cast_binding"`
	ReelBinding string       `json:"reel_binding"`
	Casts       []castReview `json:"casts"`
}

type timedInput struct {
	PressUS   int64  `json:"press_us"`
	ReleaseUS int64  `json:"release_us"`
	Kind      string `json:"kind"`
}

type timingCast struct {
	Cast           timedInput   `json:"cast"`
	ReelCandidates []timedInput `json:"reel_candidates"`
	EndUS          int64        `json:"end_us"`
	Review         *castReview  `json:"review,omitempty"`
	ReactionUS     *[2]int64    `json:"reaction_interval_us,omitempty"`
	nextCast       bool
}

type timingSession struct {
	inputtiming.Session
	Split            string         `json:"split"`
	Status           string         `json:"status"`
	CastBinding      string         `json:"cast_binding"`
	ReelBinding      string         `json:"reel_binding"`
	Casts            []timingCast   `json:"casts"`
	CandidateHolds   *distribution  `json:"candidate_reel_holds,omitempty"`
	VerifiedHolds    *distribution  `json:"verified_reel_holds,omitempty"`
	VerifiedCooldown *distribution  `json:"verified_cooldowns,omitempty"`
	Gameplay         *gameplayAudit `json:"reviewed_gameplay,omitempty"`
	LegacyBiteLabels []dataset.Cast `json:"legacy_casts_for_review_only,omitempty"`
}

func boundInput(b manualBinding, e playtrace.Event, cast bool) (timedInput, bool) {
	press, ok := b.match(e)
	if !ok && cast && b.kind == "mouse" && e.Type == "drag" {
		click := e
		click.Type = "click"
		press, ok = b.match(click)
	}
	return timedInput{press, e.ElapsedUS, e.Type}, ok
}

// pairCandidates runs on one uninterrupted interval at a time. Sorting by
// press time accounts for overlapping gestures that complete in another order.
func pairCandidates(casts, reels []timedInput, end int64) []timingCast {
	byPress := func(a, b timedInput) int { return cmp.Compare(a.PressUS, b.PressUS) }
	slices.SortStableFunc(casts, byPress)
	slices.SortStableFunc(reels, byPress)
	var pairs []timingCast
	for i, cast := range casts {
		stop := end
		if i+1 < len(casts) {
			stop = casts[i+1].PressUS
		}
		row := timingCast{Cast: cast, EndUS: stop, ReelCandidates: []timedInput{}, nextCast: i+1 < len(casts)}
		for len(reels) > 0 && reels[0].PressUS < stop {
			if reels[0].PressUS > cast.PressUS && reels[0].ReleaseUS <= stop {
				row.ReelCandidates = append(row.ReelCandidates, reels[0])
			}
			reels = reels[1:]
		}
		pairs = append(pairs, row)
	}
	return pairs
}

func readTimingSession(dir, castSpec, reelSpec string) (timingSession, []inputtiming.Sample, error) {
	s := timingSession{Session: inputtiming.Session{Name: filepath.Base(filepath.Clean(dir))}, Casts: []timingCast{}}
	var review timingReview
	raw, err := os.ReadFile(filepath.Join(dir, reviewFile))
	if err == nil {
		if err := json.Unmarshal(raw, &review); err != nil {
			return s, nil, fmt.Errorf("%s: %w", reviewFile, err)
		}
		s.ReviewSHA256 = fmt.Sprintf("%x", sha256.Sum256(raw))
		castSpec, reelSpec = review.CastBinding, review.ReelBinding
	} else if !errors.Is(err, os.ErrNotExist) {
		return s, nil, err
	}
	cast, err := parseBinding(castSpec)
	if err != nil {
		return s, nil, err
	}
	reel, err := parseBinding(reelSpec)
	if err != nil {
		return s, nil, err
	}
	if cast.kind == "" || reel.kind == "" || (cast.kind == reel.kind && strings.EqualFold(cast.value, reel.value)) {
		return s, nil, errors.New("supply distinct manual cast and reel bindings")
	}
	s.CastBinding, s.ReelBinding = castSpec, reelSpec
	file, err := os.Open(dataset.InputPath(dir))
	if err != nil {
		return s, nil, err
	}
	defer file.Close()
	hash := sha256.New()
	dec := json.NewDecoder(io.TeeReader(file, hash))
	var first json.RawMessage
	if err := dec.Decode(&first); err != nil {
		return s, nil, err
	}
	h, err := playtrace.DecodeHeader(first)
	if err != nil {
		return s, nil, err
	}
	if h.Source != playtrace.Manual {
		return s, nil, errors.New("expected a manual input trace")
	}
	var casts, reels []timedInput
	var candidateHolds []int64
	var last, start int64
	active, ended := h.Schema < 3, false
	s.Status = "no footer"
	flush := func(end int64) {
		s.Casts = append(s.Casts, pairCandidates(casts, reels, end)...)
		casts, reels, start = nil, nil, end
	}
	err = playtrace.ReadEvents(dec, func(e playtrace.Event) error {
		if ended || e.ElapsedUS < last {
			return errors.New("events after the footer or out of timestamp order")
		}
		last = e.ElapsedUS
		switch e.Type {
		case "segment_start", "segment_end", "mouse_gap", "session_end":
			flush(last)
			switch e.Type {
			case "segment_start":
				active = true
			case "segment_end":
				active = false
			case "session_end":
				s.Status, ended = e.Status, true
			}
		default:
			if !active {
				return nil
			}
			if a, ok := boundInput(cast, e, true); ok && a.PressUS >= start {
				casts = append(casts, a)
			}
			if a, ok := boundInput(reel, e, false); ok && a.PressUS >= start {
				reels = append(reels, a)
				if a.ReleaseUS > a.PressUS {
					candidateHolds = append(candidateHolds, a.ReleaseUS-a.PressUS)
				}
			}
		}
		return nil
	})
	if err != nil {
		return s, nil, err
	}
	flush(last)
	s.TraceSHA256 = fmt.Sprintf("%x", hash.Sum(nil))
	s.CandidateHolds = describe(candidateHolds)
	if legacy, err := dataset.Load(dir); err == nil {
		s.LegacyBiteLabels = legacy.Labels.Casts
	} else if !errors.Is(err, os.ErrNotExist) {
		return s, nil, err
	}
	samples, err := s.applyReview(review.Casts)
	s.Samples = len(samples)
	if err == nil && s.Status == "complete" {
		s.Gameplay = auditReviewedGameplay(s.Casts)
	}
	return s, samples, err
}

func (s *timingSession) applyReview(reviews []castReview) ([]inputtiming.Sample, error) {
	byPress := make(map[int64]*timingCast, len(s.Casts))
	for i := range s.Casts {
		byPress[s.Casts[i].Cast.PressUS] = &s.Casts[i]
	}
	for i := range reviews {
		r := &reviews[i]
		c := byPress[r.CastUS]
		if c == nil || c.Review != nil {
			return nil, fmt.Errorf("review cast %d has no unique candidate", r.CastUS)
		}
		switch r.Outcome {
		case "reeled", "missed", "no_bite", "uncertain":
		default:
			return nil, fmt.Errorf("review cast %d has invalid outcome %q", r.CastUS, r.Outcome)
		}
		if (r.Outcome == "reeled") != (r.ReelUS != nil) {
			return nil, fmt.Errorf("review cast %d: reeled outcome requires a reel and other outcomes cannot have one", r.CastUS)
		}
		if q := r.Cue; q != nil {
			if (q.Source != "visual" && q.Source != "audio" && q.Source != "simulator") ||
				q.EarliestUS < r.CastUS || q.LatestUS < q.EarliestUS || q.LatestUS > c.EndUS ||
				r.Outcome == "no_bite" || r.ReelUS != nil && q.LatestUS > *r.ReelUS {
				return nil, fmt.Errorf("review cast %d: invalid bite cue interval or source", r.CastUS)
			}
			if r.ReelUS != nil {
				c.ReactionUS = &[2]int64{*r.ReelUS - q.LatestUS, *r.ReelUS - q.EarliestUS}
			}
		}
		c.Review = r
	}
	var samples []inputtiming.Sample
	var holds, cooldowns []int64
	for i, c := range s.Casts {
		if c.Review == nil || c.Review.ReelUS == nil {
			continue
		}
		reel := slices.IndexFunc(c.ReelCandidates, func(a timedInput) bool { return a.PressUS == *c.Review.ReelUS })
		if reel < 0 {
			return nil, fmt.Errorf("review cast %d: reel %d is outside its candidate interval", c.Cast.PressUS, *c.Review.ReelUS)
		}
		a := c.ReelCandidates[reel]
		sample := inputtiming.Sample{HoldUS: a.ReleaseUS - a.PressUS}
		if sample.HoldUS == 0 {
			continue // a key press has no measured mouse hold
		}
		holds = append(holds, sample.HoldUS)
		if c.ReactionUS != nil && c.Review.Cue.Source != "audio" && inputtiming.ValidReaction(*c.ReactionUS) {
			sample.ReactionUS = c.ReactionUS
		}
		if c.nextCast {
			next := s.Casts[i+1].Review
			if next != nil && next.Outcome != "uncertain" {
				gap := next.CastUS - a.ReleaseUS
				cooldowns = append(cooldowns, gap)
				sample.CooldownUS = gap
				if !sample.Valid() {
					sample.CooldownUS = 0
				}
			}
		}
		if sample.Valid() && s.Status == "complete" {
			samples = append(samples, sample)
		}
	}
	s.VerifiedHolds, s.VerifiedCooldown = describe(holds), describe(cooldowns)
	return samples, nil
}

func readTimingCalibration(path string) (*inputtiming.Calibration, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var report struct {
		Aligned []alignedSummary `json:"aligned"`
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		return nil, err
	}
	c := inputtiming.Calibration{ReportSHA256: fmt.Sprintf("%x", sha256.Sum256(raw))}
	var samples []int64
	seen := map[string]bool{}
	for _, row := range report.Aligned {
		if row.Source != playtrace.AppDryRun || row.TimingProfile != "fixed" || row.Detector == "" || len(row.DetectorDelayUS) == 0 {
			continue
		}
		if row.GroundStatus != "complete" || row.ResponseStatus != "complete" {
			continue
		}
		if c.Detector != "" && c.Detector != row.Detector {
			return nil, errors.New("calibrate with one bite detector at a time")
		}
		if row.Session == "" || seen[row.Session] {
			return nil, errors.New("calibration needs one fixed trace per simulator session")
		}
		seen[row.Session] = true
		c.Detector = row.Detector
		for _, delay := range row.DetectorDelayUS {
			if delay < 0 || delay > 1_500_000 {
				return nil, errors.New("calibration cue-to-detector delay is out of bounds")
			}
			samples = append(samples, delay)
		}
	}
	c.Samples = len(samples)
	if c.Samples > 0 {
		c.LagUS = nearestRank(samples, 50)
	}
	if !c.Valid() {
		return nil, errors.New("calibration requires at least five aligned cues from fixed simulator dry runs")
	}
	return &c, nil
}

func runInputTiming(dirs []string, cast, reel, train, modelPath string, calibration *inputtiming.Calibration, out io.Writer) error {
	if len(dirs) == 0 {
		return errors.New("-input-timing needs recording directories")
	}
	training := map[string]bool{}
	if train != "" {
		for name := range strings.SplitSeq(train, ",") {
			training[strings.TrimSpace(name)] = true
		}
	}
	model := inputtiming.Model{Version: inputtiming.Version, Calibration: calibration}
	seen, hashes := map[string]bool{}, map[string]bool{}
	report := struct {
		FixedHoldUS  int             `json:"fixed_reel_hold_us"`
		FixedDelayUS int             `json:"fixed_reel_delay_us"`
		Sessions     []timingSession `json:"sessions"`
	}{FixedHoldUS: inputtiming.FixedHoldUS, FixedDelayUS: inputtiming.FixedDelayUS}
	for _, dir := range dirs {
		s, samples, err := readTimingSession(dir, cast, reel)
		if err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		if seen[s.Name] || hashes[s.TraceSHA256] {
			return fmt.Errorf("duplicate recording name or trace: %s", dir)
		}
		seen[s.Name], hashes[s.TraceSHA256] = true, true
		s.Split = "holdout"
		if training[s.Name] {
			s.Split = "train"
			model.Training = append(model.Training, s.Session)
			model.Samples = append(model.Samples, samples...)
		} else {
			model.Holdout = append(model.Holdout, s.Session)
		}
		report.Sessions = append(report.Sessions, s)
	}
	for name := range training {
		if !seen[name] {
			return fmt.Errorf("training recording %q was not supplied", name)
		}
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		return err
	}
	if modelPath == "" {
		return nil
	}
	if len(model.Holdout) == 0 || len(model.Samples) == 0 {
		return errors.New("model export requires verified training holds and at least one holdout recording")
	}
	return writeProfile(modelPath, model)
}

func writeProfile(modelPath string, profile any) error {
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(modelPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	return errors.Join(err, f.Close())
}
