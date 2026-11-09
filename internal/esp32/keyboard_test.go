package esp32

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"math"
	"strings"
	"testing"

	"go.bug.st/serial"
)

type testPort struct {
	serial.Port
	reply  *strings.Reader
	writes strings.Builder
	closed bool
}

func (p *testPort) Read(b []byte) (int, error)  { return p.reply.Read(b) }
func (p *testPort) Write(b []byte) (int, error) { return p.writes.Write(b) }
func (p *testPort) Close() error                { p.closed = true; return nil }

// mousePort is firmware that moves a fake pointer by gain(counts) per report.
type mousePort struct {
	serial.Port
	gain    func(counts int) float64
	x, y    int
	replies bytes.Buffer
	moves   int
}

func (p *mousePort) Read(b []byte) (int, error) { return p.replies.Read(b) }
func (p *mousePort) Write(b []byte) (int, error) {
	var dx, dy int
	if _, err := fmt.Sscanf(string(b), "MOVE %d %d\n", &dx, &dy); err != nil {
		p.replies.WriteString("ERR command\r\n")
		return len(b), nil
	}
	p.x += int(float64(dx) * p.gain(dx))
	p.y += int(float64(dy) * p.gain(dy))
	p.moves++
	p.replies.WriteString("OK\r\n")
	return len(b), nil
}

func TestMoveToReachesTargetDespiteAcceleration(t *testing.T) {
	for name, gain := range map[string]func(int) float64{
		"linear": func(int) float64 { return 1 },
		"double": func(int) float64 { return 2 },
		// Fast reports move further, like Enhance pointer precision.
		"accelerated": func(n int) float64 { return 0.5 + math.Abs(float64(n))/50 },
	} {
		t.Run(name, func(t *testing.T) {
			port := &mousePort{gain: gain, x: 900, y: 100}
			k := &Keyboard{port: port, reader: bufio.NewReader(timeoutReader{port}), pointer: true}
			pos := func() (int, int, bool) { return port.x, port.y, true }
			if err := k.MoveTo(image.Pt(120, 640), pos); err != nil {
				t.Fatalf("%v after %d moves, pointer at %d,%d", err, port.moves, port.x, port.y)
			}
			if port.moves > 20 {
				t.Errorf("took %d moves", port.moves)
			}
		})
	}
}

func TestTapClosesAfterMissingAcknowledgment(t *testing.T) {
	port := &testPort{reply: strings.NewReader("OK\r\n")}
	k := &Keyboard{port: port, reader: bufio.NewReader(timeoutReader{port})}
	if err := k.Tap(4); err != nil {
		t.Fatal(err)
	}
	if err := k.Tap(0x3a); err == nil || !port.closed {
		t.Fatalf("missing acknowledgment returned %v, closed = %v", err, port.closed)
	}
	if err := k.Tap(0x3a); err == nil || port.writes.String() != "TAP 04\nTAP 3A\n" {
		t.Fatalf("unexpected retry, writes = %q, error = %v", port.writes.String(), err)
	}
}
