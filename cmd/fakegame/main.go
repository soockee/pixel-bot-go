// Command fakegame renders a local fishing scenario for end-to-end tests.
// It shows a noisy background, a bobber, and scripted bites in a frameless
// window, one scene pixel per screen pixel. Each phase change is printed to
// stdout as a JSON line for correlation with the application logs.
//
// Usage: fakegame [-scenario file.json] [-title "Fake Game"]
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/soocke/pixel-bot-go/assets"
	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/internal/fakegame"
	"github.com/soocke/pixel-bot-go/internal/playtrace"
)

const frameInterval = 50 * time.Millisecond

// page shows /frame at one image pixel per device pixel and reloads it when
// /version changes.
const page = `<!doctype html><html><head><style>
html,body{margin:0;overflow:hidden;background:#000}
img{display:block;image-rendering:pixelated}
</style></head><body><img id="f" alt=""><script>
const img = document.getElementById("f");
let current = "";
img.onload = () => { img.style.width = img.naturalWidth / devicePixelRatio + "px"; };
setInterval(async () => {
  const v = await (await fetch("/version")).text();
  if (v !== current) { current = v; img.src = "/frame?v=" + v; }
}, 50);
</script></body></html>`

// scene holds the newest rendered frame as PNG.
type scene struct {
	mu      sync.Mutex
	png     []byte
	version int
}

func (s *scene) set(img image.Image) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		fmt.Fprintln(os.Stderr, "fakegame:", err)
		return
	}
	s.mu.Lock()
	s.png, s.version = buf.Bytes(), s.version+1
	s.mu.Unlock()
}

func (s *scene) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	data, version := s.png, s.version
	s.mu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	switch r.URL.Path {
	case "/version":
		fmt.Fprint(w, version)
	case "/frame":
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(data)
	default:
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, page)
	}
}

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
	view := &scene{}
	view.set(r.Render(fakegame.PhaseEmpty))
	game := application.New(application.Options{Name: *title, Assets: application.AssetOptions{Handler: view}})
	win := game.Window.NewWithOptions(application.WebviewWindowOptions{Title: *title, Frameless: true, DisableResize: true, URL: "/", Hidden: true})
	game.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		win.SetPhysicalBounds(application.Rect{X: px, Y: py, Width: sc.Width, Height: sc.Height})
		win.Show()
	})

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
	tick := func() bool {
		phase, done := sc.PhaseAt(time.Since(start))
		if phase != current {
			current = phase
			br := r.BobberRect()
			_ = enc.Encode(map[string]any{
				"t": time.Now().UTC().Format(time.RFC3339Nano), "phase": phase,
				"bobber": map[string]int{"x": br.Min.X, "y": br.Min.Y, "w": br.Dx(), "h": br.Dy()},
			})
			view.set(r.Render(phase))
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
					return false
				}
			}
		}
		return !done
	}
	go func() {
		for range time.Tick(frameInterval) {
			if !tick() {
				game.Quit()
				return
			}
		}
	}()
	if err := game.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "fakegame:", err)
	}
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
