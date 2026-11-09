package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/w32"
)

// trailFor is how long a pointer position stays in the drawn trail.
const trailFor = 1500 * time.Millisecond

// pointerPage draws the dry run's pointer and its recent trail. Positions are
// scene pixels, so the canvas is scaled to device pixels.
const pointerPage = `<!doctype html><html><head><style>
html,body{margin:0;overflow:hidden;background:transparent}
canvas{display:block;width:100vw;height:100vh}
</style></head><body><canvas id="c"></canvas><script>
const c = document.getElementById("c"), g = c.getContext("2d");
setInterval(async () => {
  const points = await (await fetch("/pointer")).json();
  c.width = innerWidth * devicePixelRatio; c.height = innerHeight * devicePixelRatio;
  g.clearRect(0, 0, c.width, c.height);
  g.lineWidth = 2; g.lineCap = "round";
  for (let i = 1; i < points.length; i++) {
    const [a, b] = [points[i - 1], points[i]];
    g.strokeStyle = "rgba(255, 210, 60, " + (1 - b.age / 1500) + ")"; // trailFor
    g.beginPath(); g.moveTo(a.x, a.y); g.lineTo(b.x, b.y); g.stroke();
  }
  const p = points[points.length - 1];
  if (p) { g.fillStyle = "#ffd23c"; g.beginPath(); g.arc(p.x, p.y, 5, 0, 7); g.fill(); }
}, 30);
</script></body></html>`

// pointerPoint is a pointer position in scene pixels and when it arrived.
type pointerPoint struct {
	X, Y int
	at   time.Time
}

// pointer keeps the dry run's recent pointer positions, read from the app's
// trace (-trace-out) as the app writes it.
type pointer struct {
	mu     sync.Mutex
	points []pointerPoint
}

// follow reads the "move" events the app appends to path until the program
// ends, waiting for the file to appear. Old content is skipped, so a reused
// trace does not replay.
func (p *pointer) follow(path string) {
	var file *os.File
	for {
		var err error
		if file, err = os.Open(path); err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return
	}
	r := bufio.NewReader(file)
	var line []byte
	for {
		chunk, err := r.ReadBytes('\n')
		line = append(line, chunk...)
		if errors.Is(err, io.EOF) {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		if err != nil {
			return
		}
		var e struct {
			Type     string `json:"type"`
			Position *struct {
				X int `json:"x"`
				Y int `json:"y"`
			} `json:"position"`
		}
		if json.Unmarshal(line, &e) == nil && e.Type == "move" && e.Position != nil {
			p.add(pointerPoint{X: e.Position.X, Y: e.Position.Y, at: time.Now()})
		}
		line = line[:0]
	}
}

func (p *pointer) add(pt pointerPoint) {
	p.mu.Lock()
	defer p.mu.Unlock()
	keep := 0
	for keep < len(p.points) && time.Since(p.points[keep].at) > trailFor {
		keep++
	}
	p.points = append(p.points[keep:], pt)
}

// ServeHTTP serves the trail as JSON at /pointer and the page that draws it.
func (p *pointer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path != "/pointer" {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, pointerPage)
		return
	}
	type point struct {
		X   int   `json:"x"`
		Y   int   `json:"y"`
		Age int64 `json:"age"` // milliseconds
	}
	p.mu.Lock()
	out := make([]point, 0, len(p.points))
	for _, pt := range p.points {
		if age := time.Since(pt.at); age < trailFor {
			out = append(out, point{pt.X, pt.Y, age.Milliseconds()})
		}
	}
	p.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// newPointerWindow opens a see-through window for the trail. It is titled
// differently from the game, so the bot never selects it.
func newPointerWindow(game *application.App) *application.WebviewWindow {
	return game.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "pointer", Title: "Fake Game pointer trail", URL: "/pointer.html", Hidden: true,
		Frameless: true, AlwaysOnTop: true, DisableResize: true,
		BackgroundType: application.BackgroundTypeTransparent,
		Windows:        application.WindowsWindow{HiddenOnTaskbar: true, DisableFramelessWindowDecorations: true},
	})
}

// passThrough hides win from screen capture, so the bot never sees the trail,
// and lets clicks and focus through to the game below, as app/overlay.go does
// for the app's overlay. It reports whether the native window existed yet.
func passThrough(win *application.WebviewWindow) bool {
	hwnd := w32.HWND(win.NativeWindow())
	if hwnd == 0 {
		return false
	}
	w32.SetWindowDisplayAffinity(uintptr(hwnd), w32.WDA_EXCLUDEFROMCAPTURE)
	style := uint32(w32.GetWindowLong(hwnd, w32.GWL_EXSTYLE))
	w32.SetWindowLong(hwnd, w32.GWL_EXSTYLE, style|w32.WS_EX_NOACTIVATE|w32.WS_EX_TRANSPARENT|w32.WS_EX_LAYERED)
	w32.SetLayeredWindowAttributes(hwnd, 0, 255, w32.LWA_ALPHA)
	return true
}
