package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"log/slog"
	"math"
	"os"
	"sync"
	"time"

	"github.com/soocke/pixel-bot-go/domain/capture"
	"github.com/soocke/pixel-bot-go/domain/detect"
)

// watch observes an existing bobber through PrintWindow and the real bite
// detector, then saves every watched box as a PNG named by its time.
func watch(game capture.WindowIdentity, d time.Duration, out string, load bool) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	det := detect.NewSplashSoundDetector(logger)
	var mu sync.Mutex
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	go func() {
		err := capture.CaptureAudio(ctx, game.PID, func(pcm []byte, at time.Time) error {
			var sum float64
			for i := 0; i+1 < len(pcm); i += 2 {
				v := float64(int16(binary.LittleEndian.Uint16(pcm[i:])))
				sum += v * v
			}
			if rms := math.Sqrt(sum / float64(max(len(pcm)/2, 1))); rms > 60 {
				fmt.Printf("%6d ms  sound rms %.0f\n", at.Sub(start).Milliseconds(), rms)
			}
			mu.Lock()
			defer mu.Unlock()
			if det.Hear(pcm, at) {
				fmt.Printf("%6d ms  BITE from sound\n", at.Sub(start).Milliseconds())
			}
			return nil
		})
		if err != nil && ctx.Err() == nil {
			fmt.Println("audio:", err)
		}
	}()
	var box image.Rectangle
	var frames int
	var slowest time.Duration
	var lastPoll time.Time
	for ctx.Err() == nil {
		area, err := game.Area()
		if err != nil {
			fmt.Println(err)
			return
		}
		if box.Empty() {
			f, err := game.Print(area)
			if err != nil {
				fmt.Println(err)
				return
			}
			m, found := detect.NewMarkerLocator().Locate(f.Image)
			if !found || time.Since(start) < 1500*time.Millisecond { // skip the previous cast's marker
				time.Sleep(100 * time.Millisecond)
				continue
			}
			box = m.Box.Add(f.Origin)
			mu.Lock()
			det.Reset()
			mu.Unlock()
			fmt.Printf("%6d ms  locked %v\n", time.Since(start).Milliseconds(), box)
			continue
		}
		t0 := time.Now()
		grab := box
		if load { // the app grabs the scene around the box and polls two corner signals
			m := max(box.Dx(), box.Dy())
			grab = image.Rect(box.Min.X-2*m, box.Min.Y-m, box.Max.X+2*m, box.Max.Y+m).Intersect(area)
		}
		f, err := game.Print(grab)
		if err != nil {
			fmt.Println(err)
			return
		}
		if load {
			f.Image = f.Image.SubImage(box.Sub(f.Origin)).(*image.RGBA)
			if time.Since(lastPoll) >= 250*time.Millisecond {
				lastPoll = time.Now()
				game.Print(image.Rect(8, 24, 32, 30).Add(area.Min))
				game.Print(image.Rect(8, 8, 38, 14).Add(area.Min))
			}
		}
		slowest = max(slowest, time.Since(t0))
		frames++
		mu.Lock()
		bite := det.Feed(f.Image, f.CapturedAt)
		mu.Unlock()
		ms := f.CapturedAt.Sub(start).Milliseconds()
		if bite {
			fmt.Printf("%6d ms  BITE from splash alone\n", ms)
		}
		save(out, fmt.Sprintf("%06d.png", ms), f.Image)
	}
	fmt.Printf("%d frames watched, %.1f fps, slowest grab %v\n", frames, float64(frames)/time.Since(start).Seconds(), slowest)
}
