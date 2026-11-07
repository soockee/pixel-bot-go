// Command fakegame renders a local fishing scenario for end-to-end tests.
// It shows a noisy background, a bobber, and scripted bites. Each phase change
// is printed to stdout as a JSON line for correlation with the application logs.
//
// Usage: fakegame [-scenario file.json] [-title "Fake Game"]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/soocke/pixel-bot-go/assets"
	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/internal/fakegame"
	"github.com/soocke/pixel-bot-go/internal/playtrace"
	"github.com/soocke/pixel-bot-go/ui/images"

	//lint:ignore ST1001 Dot import is intentional for concise Tk widget DSL builders.
	. "modernc.org/tk9.0"
)

const frameInterval = 50 * time.Millisecond

func main() {
	scenarioPath := flag.String("scenario", "", "path to a scenario JSON file (default: built-in loop)")
	traceOut := flag.String("trace-out", "", "new JSONL file for displayed phases and bites")
	traceSession := flag.String("trace-session", "", "label shared with other traces from this playtest")
	title := flag.String("title", "Fake Game", "window title")
	pos := flag.String("pos", "50,50", "window position x,y, relative to the chosen screen")
	screen := flag.Int("screen", 0, "monitor to open on: 1 = primary, 2 = next from the left (0 = primary origin)")
	flag.Parse()

	sc := fakegame.DefaultScenario()
	if *scenarioPath != "" {
		var err error
		if sc, err = fakegame.LoadScenario(*scenarioPath); err != nil {
			fmt.Fprintln(os.Stderr, "fakegame:", err)
			os.Exit(1)
		}
	}
	tmpl, err := assets.FishingTargetImage()
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakegame:", err)
		os.Exit(1)
	}
	r := fakegame.NewRenderer(sc, tmpl)

	App.WmTitle(*title)
	var px, py int
	if _, err := fmt.Sscanf(*pos, "%d,%d", &px, &py); err != nil {
		fmt.Fprintln(os.Stderr, "fakegame: bad -pos, want x,y")
		os.Exit(1)
	}
	if b, ok := action.ScreenBounds(*screen); ok {
		px, py = px+b.Min.X, py+b.Min.Y
	} else if *screen > 0 {
		fmt.Fprintln(os.Stderr, "fakegame: screen not found:", *screen)
		os.Exit(1)
	}
	WmGeometry(App, fmt.Sprintf("+%d+%d", px, py))
	photo := NewPhoto(Data(images.EncodePNG(r.Render(fakegame.PhaseEmpty))))
	label := Label(Image(photo), Borderwidth(0))
	Pack(label)

	start := time.Now()
	var trace *playtrace.Writer
	if *traceOut != "" {
		utc := start.UTC()
		trace, err = playtrace.Create(*traceOut, playtrace.Header{
			Source: playtrace.FakeGame, Clock: playtrace.Monotonic, SessionID: *traceSession, StartedUTC: &utc,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "fakegame trace:", err)
			os.Exit(1)
		}
	}
	biteID := 0
	var traceErr error
	current := fakegame.Phase("")
	enc := json.NewEncoder(os.Stdout)
	var tick func()
	tick = func() {
		phase, done := sc.PhaseAt(time.Since(start))
		if phase != current {
			current = phase
			br := r.BobberRect()
			_ = enc.Encode(map[string]any{
				"t": time.Now().UTC().Format(time.RFC3339Nano), "phase": phase,
				"bobber": map[string]int{"x": br.Min.X, "y": br.Min.Y, "w": br.Dx(), "h": br.Dy()},
			})
			next := NewPhoto(Data(images.EncodePNG(r.Render(phase))))
			label.Configure(Image(next))
			photo.Delete()
			photo = next
			if trace != nil && traceErr == nil {
				at := time.Since(start).Microseconds()
				seed := json.Number(strconv.FormatInt(sc.Seed, 10))
				row := playtrace.Event{Type: "phase", ElapsedUS: at, Phase: string(phase), Seed: seed,
					Position: &playtrace.Position{X: int32(br.Min.X), Y: int32(br.Min.Y), Width: int32(sc.Width), Height: int32(sc.Height)}}
				traceErr = trace.Write(row)
				if traceErr == nil && phase == fakegame.PhaseSplash {
					id := biteID
					biteID++
					traceErr = trace.Write(playtrace.Event{Type: playtrace.BiteAvailable, ElapsedUS: at, EventID: &id, Seed: seed})
				}
				if traceErr == nil {
					traceErr = trace.Flush()
				}
				if traceErr != nil {
					fmt.Fprintln(os.Stderr, "fakegame trace:", traceErr)
					Destroy(App)
					return
				}
			}
		}
		if done {
			Destroy(App)
			return
		}
		TclAfter(frameInterval, tick)
	}
	TclAfter(frameInterval, tick)
	App.Wait()
	if trace != nil {
		status := "complete"
		if traceErr != nil {
			status = "incomplete"
		}
		if err := trace.Close(playtrace.End{ElapsedUS: time.Since(start).Microseconds(), Status: status}); err != nil {
			fmt.Fprintln(os.Stderr, "fakegame trace:", err)
			os.Exit(1)
		}
	}
}
