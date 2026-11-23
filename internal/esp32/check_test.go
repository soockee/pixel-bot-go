package esp32

import (
	"bufio"
	"fmt"
	"testing"
	"time"
)

// pingPort answers PING with PONG connected, except the answer numbers in drop.
type pingPort struct {
	mousePort
	pings int
	drop  map[int]bool
}

func (p *pingPort) Write(b []byte) (int, error) {
	if string(b) == "PING\n" {
		p.pings++
		if !p.drop[p.pings] {
			p.replies.WriteString("PONG connected\r\n")
		}
	}
	return len(b), nil
}
func (p *pingPort) ResetInputBuffer() error { p.replies.Reset(); return nil }

func TestPingCountsAnswers(t *testing.T) {
	for _, drop := range []map[int]bool{nil, {3: true}} {
		port := &pingPort{drop: drop}
		k := &Board{port: port, reader: bufio.NewReader(timeoutReader{port})}
		r, err := k.Ping(10)
		if err != nil {
			t.Fatal(err)
		}
		if want := 10 - len(drop); r.Sent != 10 || r.Answered != want || !r.Connected || r.Slowest < r.Typical {
			t.Errorf("drop %v: report %+v, want %d of 10 answered and connected", drop, r, want)
		}
	}
}

// edgePort is a pointer that stops at x = edge, as at a screen's border, or
// never moves when dead.
type edgePort struct {
	mousePort
	edge int
	dead bool
}

func (p *edgePort) Write(b []byte) (int, error) {
	var dx, dy int
	if _, err := fmt.Sscanf(string(b), "MOVE %d %d\n", &dx, &dy); err != nil {
		return 0, err
	}
	if !p.dead {
		p.x = min(p.x+dx, p.edge)
	}
	p.replies.WriteString("OK\r\n")
	return len(b), nil
}

func TestNudgeReachesWindowsAndComesBack(t *testing.T) {
	nudgeWait = 50 * time.Millisecond
	defer func() { nudgeWait = time.Second }()
	tests := []struct {
		name    string
		port    *edgePort
		wantErr bool
	}{
		{"free pointer", &edgePort{mousePort: mousePort{x: 500}, edge: 1000}, false},
		{"pointer at the right edge", &edgePort{mousePort: mousePort{x: 1000}, edge: 1000}, false},
		{"reports never arrive", &edgePort{mousePort: mousePort{x: 500}, edge: 1000, dead: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := &Board{port: tt.port, reader: bufio.NewReader(timeoutReader{tt.port}), pointer: true}
			start := tt.port.x
			_, err := k.Nudge(func() (int, int, bool) { return tt.port.x, tt.port.y, true })
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, want error %v", err, tt.wantErr)
			}
			if tt.port.x != start {
				t.Errorf("pointer left at x = %d, want back at %d", tt.port.x, start)
			}
		})
	}
}
