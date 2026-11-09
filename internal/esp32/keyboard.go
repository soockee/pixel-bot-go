// Package esp32 sends bounded keyboard taps, pointer moves and right clicks to
// the Go Fish BLE firmware.
package esp32

import (
	"bufio"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"
)

// PortName validates a Windows COM port and returns its canonical spelling.
// An empty value leaves the hardware backend disabled.
func PortName(name string) (string, error) {
	name = strings.ToUpper(strings.TrimSpace(name))
	if name == "" {
		return "", nil
	}
	digits, ok := strings.CutPrefix(name, "COM")
	n, err := strconv.ParseUint(digits, 10, 16)
	if !ok || err != nil || n == 0 {
		return "", errors.New("enter a COM port such as COM3")
	}
	return fmt.Sprintf("COM%d", n), nil
}

// SupportsKey reports whether the backend accepts this Windows virtual key.
func SupportsKey(vk byte) bool {
	return vk >= 'A' && vk <= 'Z' || vk >= '0' && vk <= '9' || vk >= 0x70 && vk <= 0x7b
}

// Keyboard owns one port for the duration of a fishing run.
type Keyboard struct {
	mu     sync.Mutex
	port   serial.Port
	reader *bufio.Reader
}

// Open checks the firmware and Bluetooth connection without sending any input.
func (k *Keyboard) Open(name string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.port != nil {
		return errors.New("ESP32 port is already open")
	}
	port, err := serial.Open(name, &serial.Mode{
		BaudRate: 115200, InitialStatusBits: &serial.ModemOutputBits{},
	})
	if err != nil {
		return fmt.Errorf("open ESP32 on %s: %w", name, err)
	}
	k.port = port
	k.reader = bufio.NewReaderSize(timeoutReader{port}, 64)
	if err := k.handshake(); err != nil {
		k.closePort()
		return fmt.Errorf("ESP32 on %s: %w", name, err)
	}
	return nil
}

func (k *Keyboard) handshake() error {
	if err := k.port.SetReadTimeout(300 * time.Millisecond); err != nil {
		return err
	}
	if err := k.port.ResetInputBuffer(); err != nil {
		return err
	}
	reply, err := k.exchange("HELLO")
	if err != nil {
		return fmt.Errorf("firmware handshake failed: %w", err)
	}
	switch reply {
	case "GOFISH 2 READY":
		return nil
	case "GOFISH 2 WAITING":
		return errors.New("connect Go Fish ESP32 Probe in Windows Bluetooth settings")
	case "GOFISH 1 READY", "GOFISH 1 WAITING":
		return errors.New("flash the current firmware, this one has no mouse")
	default:
		return fmt.Errorf("unexpected firmware response %q", reply)
	}
}

// Tap waits for acknowledgment of a bounded press and release. A failed
// transaction closes the port because the press might already have happened.
func (k *Keyboard) Tap(usage byte) error {
	if usage < 0x04 || usage > 0x65 {
		return errors.New("ESP32 requires a keyboard key")
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.send(fmt.Sprintf("TAP %02X", usage))
}

// ClickRight presses and releases the right mouse button, with the same
// failure contract as Tap.
func (k *Keyboard) ClickRight() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.send("CLICK")
}

// maxMoveSteps bounds MoveTo; a path within the game window needs about ten.
const maxMoveSteps = 40

// MoveTo steers the pointer onto target with relative mouse reports. Windows
// scales each report by its pointer speed and acceleration, so every step reads
// the real position from pos and divides the rest of the way by the gain the
// previous step showed.
func (k *Keyboard) MoveTo(target image.Point, pos func() (x, y int, ok bool)) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	gain := [2]float64{1, 1}
	for range maxMoveSteps {
		x, y, ok := pos()
		if !ok {
			return errors.New("cursor position unavailable")
		}
		rest := [2]int{target.X - x, target.Y - y}
		if rest == [2]int{} {
			return nil
		}
		var step [2]int
		for i := range step {
			step[i] = reportStep(rest[i], gain[i])
		}
		if err := k.send(fmt.Sprintf("MOVE %d %d", step[0], step[1])); err != nil {
			return err
		}
		nx, ny := settle(x, y, pos)
		moved := [2]int{nx - x, ny - y}
		for i := range gain {
			if step[i] != 0 && moved[i] != 0 && (moved[i] > 0) == (step[i] > 0) {
				gain[i] = min(max(float64(moved[i])/float64(step[i]), 0.25), 4)
			}
		}
	}
	return errors.New("ESP32 pointer did not reach the target")
}

// reportStep is the report value expected to cover rest pixels, at least one
// and at most 127 counts.
func reportStep(rest int, gain float64) int {
	if rest == 0 {
		return 0
	}
	step := int(float64(rest)/gain + 0.5*float64(sign(rest)))
	if step == 0 {
		step = sign(rest)
	}
	return min(max(step, -127), 127)
}

func sign(n int) int {
	if n < 0 {
		return -1
	}
	return 1
}

// settle waits until Windows applies the report that was just acknowledged,
// which lags the reply by up to one Bluetooth connection interval.
func settle(x, y int, pos func() (int, int, bool)) (int, int) {
	deadline := time.Now().Add(50 * time.Millisecond)
	for {
		nx, ny, ok := pos()
		if ok && (nx != x || ny != y) || time.Now().After(deadline) {
			return nx, ny
		}
		time.Sleep(time.Millisecond)
	}
}

// send runs one acknowledged command and closes the port on any other
// outcome, because the input might already have happened. The caller holds mu.
func (k *Keyboard) send(command string) error {
	if k.port == nil {
		return errors.New("ESP32 port is closed")
	}
	reply, err := k.exchange(command)
	if err == nil && reply == "OK" {
		return nil
	}
	k.closePort()
	if err != nil {
		return fmt.Errorf("ESP32 %s acknowledgment: %w", command, err)
	}
	return fmt.Errorf("ESP32 rejected %s: %q", command, reply)
}

// Close releases the port. The firmware owns the release of any pending tap.
func (k *Keyboard) Close() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.closePort()
}

func (k *Keyboard) closePort() {
	if k.port != nil {
		_ = k.port.Close()
		k.port, k.reader = nil, nil
	}
}

func (k *Keyboard) exchange(command string) (string, error) {
	data := command + "\n"
	n, err := io.WriteString(k.port, data)
	if err != nil {
		return "", err
	}
	if n != len(data) {
		return "", io.ErrShortWrite
	}
	reply, err := k.reader.ReadSlice('\n')
	return strings.TrimRight(string(reply), "\r\n"), err
}

type timeoutReader struct{ io.Reader }

func (r timeoutReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if n == 0 && err == nil {
		return 0, os.ErrDeadlineExceeded
	}
	return n, err
}
