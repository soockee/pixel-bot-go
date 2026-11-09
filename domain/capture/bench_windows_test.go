package capture

import (
	"fmt"
	"image"
	"testing"
)

// BenchmarkGrab times a whole capture at the sizes the engine uses: a bobber
// box while watching and a game window while searching.
func BenchmarkGrab(b *testing.B) {
	for _, size := range []int{64, 200, 1280} {
		area := image.Rect(0, 0, size, size*9/16+1)
		b.Run(fmt.Sprintf("%dx%d", area.Dx(), area.Dy()), func(b *testing.B) {
			for b.Loop() {
				if _, err := Grab(area); err != nil {
					b.Skip(err)
				}
			}
		})
	}
}
