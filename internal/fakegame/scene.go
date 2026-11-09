// Package fakegame renders synthetic scenes for end-to-end tests. Scenes
// contain a noisy background, an optional target template, and a scripted splash.
package fakegame

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math/rand"
	"os"
	"time"

	"github.com/soocke/pixel-bot-go/domain/detect"
)

// Phase is what the fake game shows during a timeline segment.
type Phase string

const (
	// PhaseEmpty shows only the background (no bobber in the water).
	PhaseEmpty Phase = "empty"
	// PhaseBobber shows the static bobber.
	PhaseBobber Phase = "bobber"
	// PhaseSplash shows the bobber with a bright splash patch (a bite).
	PhaseSplash Phase = "splash"
)

// Segment is one step of the scripted timeline.
type Segment struct {
	Phase Phase `json:"phase"`
	MS    int   `json:"ms"`
}

// Scenario describes the scripted timeline and scene geometry.
type Scenario struct {
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	BobberX   int       `json:"bobber_x"`     // top-left of the bobber in the scene
	BobberY   int       `json:"bobber_y"`     //
	Scale     float64   `json:"bobber_scale"` // template scale factor
	Seed      int64     `json:"seed"`
	Loop      bool      `json:"loop"`
	Timeline  []Segment `json:"timeline"`
	SplashLum byte      `json:"splash_lum"`
	// Spread lands each new bobber at a random spot up to this many pixels
	// from BobberX, BobberY, so the pointer has new ways to go; 0 keeps it still.
	Spread int `json:"bobber_spread"`
	// Sound is what the game plays: SoundBite (the default) a quiet hum and a
	// recorded bite sound 200 to 450 ms after each splash, SoundNoBite the hum
	// only, as with sound effects off and music on, SoundOff nothing.
	Sound string `json:"sound"`
}

// The Scenario.Sound values.
const (
	SoundBite   = "bite"
	SoundNoBite = "no_bite"
	SoundOff    = "off"
)

// DefaultScenario returns a looping cast: empty, bobber, bite, empty.
func DefaultScenario() Scenario {
	return Scenario{
		Width: 480, Height: 320, BobberX: 200, BobberY: 120, Scale: 1.0, Seed: 1, Loop: true, SplashLum: 230, Sound: SoundBite,
		Timeline: []Segment{
			{PhaseEmpty, 2000}, {PhaseBobber, 4000}, {PhaseSplash, 600}, {PhaseEmpty, 3000},
		},
	}
}

// LoadScenario reads a scenario from a JSON file; missing fields keep defaults.
func LoadScenario(path string) (Scenario, error) {
	sc := DefaultScenario()
	data, err := os.ReadFile(path)
	if err != nil {
		return sc, err
	}
	if err := json.Unmarshal(data, &sc); err != nil {
		return sc, fmt.Errorf("parse scenario: %w", err)
	}
	if sc.Sound != SoundBite && sc.Sound != SoundNoBite && sc.Sound != SoundOff {
		return sc, fmt.Errorf("scenario sound %q: want %s, %s or %s", sc.Sound, SoundBite, SoundNoBite, SoundOff)
	}
	if sc.Width <= 0 || sc.Height <= 0 || len(sc.Timeline) == 0 {
		return sc, fmt.Errorf("scenario needs positive width/height and a timeline")
	}
	return sc, nil
}

// PhaseAt returns the phase active at elapsed time, and whether the script ended.
func (s Scenario) PhaseAt(elapsed time.Duration) (Phase, bool) {
	total := time.Duration(0)
	for _, seg := range s.Timeline {
		total += time.Duration(seg.MS) * time.Millisecond
	}
	if total <= 0 {
		return PhaseEmpty, true
	}
	if elapsed >= total {
		if !s.Loop {
			return s.Timeline[len(s.Timeline)-1].Phase, true
		}
		elapsed %= total
	}
	for _, seg := range s.Timeline {
		d := time.Duration(seg.MS) * time.Millisecond
		if elapsed < d {
			return seg.Phase, false
		}
		elapsed -= d
	}
	return PhaseEmpty, false
}

// Renderer draws scenes. The background is rendered once and reused, so
// frames within a phase are identical (no spurious motion).
type Renderer struct {
	sc   Scenario
	tmpl image.Image
	bg   *image.RGBA
	at   image.Point // the bobber's top-left, see Land
}

// NewRenderer builds a renderer for the scenario and bobber template.
func NewRenderer(sc Scenario, tmpl image.Image) *Renderer {
	rng := rand.New(rand.NewSource(sc.Seed))
	bg := image.NewRGBA(image.Rect(0, 0, sc.Width, sc.Height))
	for y := 0; y < sc.Height; y++ {
		for x := 0; x < sc.Width; x++ {
			v := uint8(70 + rng.Intn(30))
			bg.SetRGBA(x, y, color.RGBA{v, v + 10, v + 25, 255})
		}
	}
	return &Renderer{sc: sc, tmpl: tmpl, bg: bg, at: image.Pt(sc.BobberX, sc.BobberY)}
}

// BobberRect is the on-scene rectangle covered by the scaled bobber.
func (r *Renderer) BobberRect() image.Rectangle {
	b := r.tmpl.Bounds()
	w := int(float64(b.Dx()) * r.sc.Scale)
	h := int(float64(b.Dy()) * r.sc.Scale)
	return image.Rect(r.at.X, r.at.Y, r.at.X+w, r.at.Y+h)
}

// Land moves the bobber to a random spot within the scenario's spread of its
// home, keeping the addon's brackets inside the scene. Call it when a new cast
// begins; with no spread the bobber stays where it is.
func (r *Renderer) Land(rng *rand.Rand) {
	if r.sc.Spread <= 0 {
		return
	}
	size := r.BobberRect().Size()
	pick := func(home, extent, scene int) int {
		lo, hi := max(24, home-r.sc.Spread), min(scene-extent-24, home+r.sc.Spread)
		if hi <= lo {
			return home
		}
		return lo + rng.Intn(hi-lo+1)
	}
	r.at = image.Pt(pick(r.sc.BobberX, size.X, r.sc.Width), pick(r.sc.BobberY, size.Y, r.sc.Height))
}

// MarkerRect is the area the addon's brackets surround.
func (r *Renderer) MarkerRect() image.Rectangle { return r.BobberRect().Inset(-24) }

// Render returns a fresh frame for the phase.
func (r *Renderer) Render(p Phase) *image.RGBA {
	out := image.NewRGBA(r.bg.Bounds())
	copy(out.Pix, r.bg.Pix)
	if p == PhaseEmpty {
		return out
	}
	br := r.BobberRect()
	tb := r.tmpl.Bounds()
	for y := br.Min.Y; y < br.Max.Y; y++ {
		for x := br.Min.X; x < br.Max.X; x++ {
			if !image.Pt(x, y).In(out.Bounds()) {
				continue
			}
			sx := tb.Min.X + (x-br.Min.X)*tb.Dx()/br.Dx()
			sy := tb.Min.Y + (y-br.Min.Y)*tb.Dy()/br.Dy()
			cr, cg, cb, ca := r.tmpl.At(sx, sy).RGBA()
			if ca == 0 {
				continue
			}
			a := ca >> 8
			i := out.PixOffset(x, y)
			for c, v := range [3]uint32{cr >> 8, cg >> 8, cb >> 8} {
				out.Pix[i+c] = uint8((v*a + uint32(out.Pix[i+c])*(255-a)) / 255)
			}
		}
	}
	if p == PhaseSplash {
		// Bright patch over and around the bobber, inside the marker box as a
		// real splash is, so the water around the bobber stays calm.
		c := image.Pt((br.Min.X+br.Max.X)/2, (br.Min.Y+br.Max.Y)/2)
		patch := image.Rect(c.X-16, c.Y-16, c.X+16, c.Y+16).Intersect(out.Bounds())
		for y := patch.Min.Y; y < patch.Max.Y; y++ {
			for x := patch.Min.X; x < patch.Max.X; x++ {
				i := out.PixOffset(x, y)
				out.Pix[i], out.Pix[i+1], out.Pix[i+2] = r.sc.SplashLum, r.sc.SplashLum, r.sc.SplashLum
			}
		}
	}
	detect.DrawMarker(out, r.MarkerRect())
	return out
}
