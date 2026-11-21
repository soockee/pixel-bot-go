package esp32

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// PingReport is what Ping measured over the USB serial link.
type PingReport struct {
	Sent, Answered   int
	Typical, Slowest time.Duration // median and longest round trip of the answers
	Connected        bool          // the last answer said Bluetooth is connected
}

// Ping sends PING n times and times each PONG. It sends no input. A missed
// answer is dropped from the read buffer, so a late PONG is not taken for the
// next one.
func (k *Keyboard) Ping(n int) (PingReport, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	r := PingReport{Sent: n}
	if k.port == nil {
		return r, errors.New("ESP32 port is closed")
	}
	var trips []time.Duration
	for range n {
		start := time.Now()
		reply, err := k.exchange("PING")
		switch {
		case err == nil && (reply == "PONG connected" || reply == "PONG waiting"):
			trips = append(trips, time.Since(start))
			r.Connected = reply == "PONG connected"
		default:
			_ = k.port.ResetInputBuffer()
			k.reader.Reset(timeoutReader{k.port})
		}
	}
	r.Answered = len(trips)
	if len(trips) > 0 {
		slices.Sort(trips)
		r.Typical, r.Slowest = trips[len(trips)/2], trips[len(trips)-1]
	}
	return r, nil
}

// nudgeWait is how long Nudge waits for Windows to apply one report, well
// past reportLag, since a link that needs longer is the problem it reports.
// Tests shorten it.
var nudgeWait = time.Second

// Nudge moves the pointer one count and back through the board, to check that
// its Bluetooth reports reach Windows, and returns how long the first took to
// show in pos. It tries the other direction when the pointer sits at a screen
// edge. Call PreparePointer first.
func (k *Keyboard) Nudge(pos func() (x, y int, ok bool)) (time.Duration, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.port == nil || !k.pointer {
		return 0, errors.New("ESP32 pointer support is unavailable")
	}
	x, y, ok := pos()
	if !ok {
		return 0, errors.New("cursor position unavailable")
	}
	for _, step := range []int{1, -1} {
		start := time.Now()
		if err := k.sendAction(fmt.Sprintf("MOVE %d 0", step), "pointer move"); err != nil {
			return 0, err
		}
		if moved(x, y, pos, nudgeWait) {
			lag := time.Since(start)
			if err := k.sendAction(fmt.Sprintf("MOVE %d 0", -step), "pointer move"); err != nil {
				return lag, err
			}
			moved(x+step, y, pos, nudgeWait) // let the way back land before the next test
			return lag, nil
		}
	}
	return 0, errors.New("the board's pointer moves do not reach Windows")
}

// moved waits up to wait for the pointer to leave x, y.
func moved(x, y int, pos func() (int, int, bool), wait time.Duration) bool {
	for deadline := time.Now().Add(wait); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		if nx, ny, ok := pos(); ok && (nx != x || ny != y) {
			return true
		}
	}
	return false
}
