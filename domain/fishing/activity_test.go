package fishing

import (
	"image"
	"image/color"
	"math/rand/v2"
	"testing"
)

func TestWanderPathDriftsAShortSmoothWay(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	start := image.Pt(500, 400)
	for range 200 {
		path := wanderPath(start, rng.IntN)
		if len(path) < 10 || len(path) > 16 {
			t.Fatalf("%d points", len(path))
		}
		end := path[len(path)-1].Sub(start)
		if d := end.X*end.X + end.Y*end.Y; d < 38*38 || d > 162*162 {
			t.Fatalf("ends %v from the start", end)
		}
		prev := start
		for _, p := range path {
			if step := p.Sub(prev); step.X*step.X+step.Y*step.Y > 45*45 {
				t.Fatalf("jumps %v in one step", step)
			}
			prev = p
		}
	}
}

func TestInWorldReadsTheAddonBlocks(t *testing.T) {
	img := image.NewRGBA(WorldSignalArea.Sub(WorldSignalArea.Min))
	if InWorld(img) {
		t.Fatal("a black capture counted as in world")
	}
	for i, c := range []color.RGBA{{250, 4, 251, 255}, {3, 252, 0, 255}, {255, 0, 255, 255}} {
		for x := i * 6; x < i*6+6; x++ {
			for y := range 6 {
				img.SetRGBA(x, y, c)
			}
		}
	}
	if !InWorld(img) {
		t.Fatal("the addon's blocks did not count as in world")
	}
	img.SetRGBA(9, 3, color.RGBA{255, 0, 255, 255}) // the middle block turns magenta
	if InWorld(img) {
		t.Fatal("a wrong middle block counted as in world")
	}
}
