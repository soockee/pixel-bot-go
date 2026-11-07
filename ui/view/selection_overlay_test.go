package view

import (
	"image"
	"testing"
)

func TestParseGeometrySelNegativeOrigin(t *testing.T) {
	cases := map[string]image.Rectangle{
		"800x600+100+50":    image.Rect(100, 50, 900, 650),
		"560x420+-1890+178": image.Rect(-1890, 178, -1330, 598),
		"100x100+-5+-7":     image.Rect(-5, -7, 95, 93),
	}
	for in, want := range cases {
		if got, ok := parseGeometrySel(in); !ok || got != want {
			t.Errorf("parseGeometrySel(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
}
