// Command fakegame opens a window that imitates the fishing scene so Pixel Bot
// can be exercised end to end without the real game. The window shows a noisy
// background, a bobber and scripted bites, and prints each phase change to
// stdout as a JSON line so a test run can be correlated with the bot's logs.
//
// Usage: fakegame [-scenario file.json] [-title "Fake Game"]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/soocke/pixel-bot-go/assets"
	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/internal/fakegame"
	"github.com/soocke/pixel-bot-go/ui/images"

	//lint:ignore ST1001 Dot import is intentional for concise Tk widget DSL builders.
	. "modernc.org/tk9.0"
)

const frameInterval = 50 * time.Millisecond

func main() {
	scenarioPath := flag.String("scenario", "", "path to a scenario JSON file (default: built-in loop)")
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
		}
		if done {
			Destroy(App)
			return
		}
		TclAfter(frameInterval, tick)
	}
	TclAfter(frameInterval, tick)
	App.Wait()
}
