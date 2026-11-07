package dataset

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/internal/playtrace"
)

var testOptions = AutoLabelOptions{Cast: "mouse:x1", Reel: "mouse:right", ReactionMS: 400, BoxPx: 40}

func click(ms int64, button string, x, y int32) playtrace.Event {
	return playtrace.Event{Type: "click", ElapsedUS: ms * 1000, Button: button, Position: &playtrace.Position{X: x, Y: y, Width: 800, Height: 600}}
}

func keyDown(ms int64, name, chord string, repeat bool) playtrace.Event {
	return playtrace.Event{Type: "key_down", ElapsedUS: ms * 1000, Key: &playtrace.Key{Name: name, Chord: chord, Repeat: repeat}}
}

// writeSession creates a 200x100 session; client (500,300) is frame (50,30).
func writeSession(t *testing.T, events ...playtrace.Event) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(FramesPath(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(FramesPath(dir), "000000100.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, 200, 100))); err != nil {
		t.Fatal(err)
	}
	if err := WriteSessionInfo(dir, SessionInfo{ClientToFrame: image.Pt(-450, -270)}); err != nil {
		t.Fatal(err)
	}
	utc := time.Now().UTC()
	trace, err := playtrace.Create(InputPath(dir), playtrace.Header{Source: playtrace.Manual, Clock: playtrace.Monotonic, StartedUTC: &utc})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if err := trace.Write(e); err != nil {
			t.Fatal(err)
		}
	}
	if err := trace.Close(playtrace.End{Status: "complete"}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestAutoLabelPairsCastsAndReels(t *testing.T) {
	dir := writeSession(t,
		click(5000, "x1", 10, 10),
		click(12000, "right", 500, 300),
		click(20000, "x1", 10, 10),
		click(29000, "left", 1, 1), // ignored
		click(30000, "right", 510, 310),
	)

	got, skipped, err := AutoLabel(dir, testOptions)

	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 0 {
		t.Errorf("skipped = %v", skipped)
	}
	biteA, biteB := int64(11600), int64(29600)
	want := Labels{
		Casts: []Cast{
			{Span: Span{7000, 12000}, Bobber: Rect{30, 10, 40, 40}, BiteMS: &biteA},
			{Span: Span{22000, 30000}, Bobber: Rect{40, 20, 40, 40}, BiteMS: &biteB},
		},
		Empty: []Span{{0, 5000}, {13000, 20000}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestCastPressThatMovedStillCounts(t *testing.T) {
	cast := click(5000, "x1", 10, 10)
	cast.Type = "drag"
	camera := click(9000, "right", 1, 1)
	camera.Type = "drag" // turns the camera, so it is not a reel
	dir := writeSession(t, cast, camera, click(12000, "right", 500, 300))

	got, skipped, err := AutoLabel(dir, testOptions)

	if err != nil || len(skipped) != 0 || len(got.Casts) != 1 || got.Casts[0].EndMS != 12000 {
		t.Errorf("got %+v skipped=%v err=%v", got, skipped, err)
	}
}

func TestAutoLabelReadsKeyBindings(t *testing.T) {
	dir := writeSession(t,
		keyDown(1000, "F3", "", true), // a held key is not a cast
		keyDown(5000, "F3", "", false),
		keyDown(12000, "B", "Shift+B", false),
	)
	opts := testOptions
	opts.Cast, opts.Reel = "key:f3", "mouse:right"
	if _, _, err := AutoLabel(dir, opts); err == nil {
		t.Fatal("key reel has no position, want no pairs")
	}

	dir = writeSession(t,
		keyDown(5000, "B", "Shift+B", false),
		keyDown(6000, "B", "", false), // plain B is another binding
		click(12000, "right", 500, 300),
	)
	opts.Cast = "key:Shift+B"
	got, skipped, err := AutoLabel(dir, opts)
	if err != nil || len(skipped) != 0 || len(got.Casts) != 1 || got.Casts[0].StartMS != 7000 {
		t.Errorf("got %+v skipped=%v err=%v", got, skipped, err)
	}
}

func TestAutoLabelSkipsWhatItCannotPlace(t *testing.T) {
	dir := writeSession(t,
		click(5000, "x1", 0, 0),
		click(9000, "x1", 0, 0), // the first cast never got a reel
		click(16000, "right", 500, 300),
		click(20000, "x1", 0, 0),
		click(21000, "right", 500, 300), // too soon to be a bite
		click(30000, "x1", 0, 0),
		click(40000, "right", 5, 5), // outside the frame
		click(50000, "x1", 0, 0),
		click(60000, "right", 500, 300),
		click(70000, "x1", 0, 0), // no reel before the end
	)

	got, skipped, err := AutoLabel(dir, testOptions)

	if err != nil {
		t.Fatal(err)
	}
	if len(got.Casts) != 2 {
		t.Errorf("casts = %+v", got.Casts)
	}
	joined := strings.Join(skipped, "; ")
	for _, want := range []string{"cast at 5000", "reel at 21000", "reel at 40000", "cast at 70000"} {
		if !strings.Contains(joined, want) {
			t.Errorf("skipped = %q, want it to mention %q", joined, want)
		}
	}
}

func TestAutoLabelReportsAMissingSetup(t *testing.T) {
	tests := []struct {
		name string
		dir  func(t *testing.T) string
		opts AutoLabelOptions
		want string
	}{
		{"bad binding", writeSessionEmpty, AutoLabelOptions{Cast: "x1", Reel: "mouse:right"}, "binding"},
		{"no pairs", writeSessionEmpty, testOptions, "no cast and reel pairs"},
		{"frames-only session", func(t *testing.T) string { return t.TempDir() }, testOptions, "-window"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := AutoLabel(tt.dir(t), tt.opts); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func writeSessionEmpty(t *testing.T) string { return writeSession(t) }

func TestWriteLabelsKeepsExistingLabels(t *testing.T) {
	dir := t.TempDir()
	writeLabels(t, dir, `{"casts":[]}`)
	if err := WriteLabels(dir, Labels{}); err == nil {
		t.Fatal("want an error")
	}
	data, err := os.ReadFile(filepath.Join(dir, labelsFile))
	if err != nil || string(data) != `{"casts":[]}` {
		t.Errorf("labels changed: %q err=%v", data, err)
	}
}

func TestWrittenLabelsLoad(t *testing.T) {
	dir := writeSession(t, click(5000, "x1", 0, 0), click(12000, "right", 500, 300))
	labels, _, err := AutoLabel(dir, testOptions)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteLabels(dir, labels); err != nil {
		t.Fatal(err)
	}
	d, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Labels.Casts) != 1 {
		t.Errorf("casts = %+v", d.Labels.Casts)
	}
}
