// Package esp32 sends bounded keyboard taps, camera drags, pointer moves and
// right clicks to the Go Fish BLE firmware.
package esp32

import (
	"bufio"
	"errors"
	"fmt"
	"image"
	"io"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

// AutoPort asks Open to find the board among the USB serial ports.
const AutoPort = "AUTO"

// PortName validates a Windows COM port, or AutoPort, and returns its canonical
// spelling. An empty value leaves the hardware backend disabled.
func PortName(name string) (string, error) {
	name = strings.ToUpper(strings.TrimSpace(name))
	if name == "" || name == AutoPort {
		return name, nil
	}
	digits, ok := strings.CutPrefix(name, "COM")
	n, err := strconv.ParseUint(digits, 10, 16)
	if !ok || err != nil || n == 0 {
		return "", errors.New("enter a COM port such as COM3, or auto to find the board")
	}
	return fmt.Sprintf("COM%d", n), nil
}

// Windows virtual keys of the mouse side buttons, which the board presses as
// HID mouse buttons 4 and 5.
const (
	vkMouse4 = 0x05
	vkMouse5 = 0x06
)

// SupportsKey reports whether the backend accepts this Windows virtual key:
// letters, digits, F1 to F12, and Mouse 4 and 5.
func SupportsKey(vk byte) bool {
	return vk >= 'A' && vk <= 'Z' || vk >= '0' && vk <= '9' || vk >= 0x70 && vk <= 0x7b ||
		vk == vkMouse4 || vk == vkMouse5
}

// Board is the paired ESP32. It owns one serial port while open.
type Board struct {
	mu      sync.Mutex
	port    serial.Port
	reader  *bufio.Reader
	timed   bool
	camera  bool
	pointer bool                                            // MOVE and CLICK, for the right-click reel
	buttons bool                                            // BUTTON, for bindings on Mouse 4 and 5
	name    string                                          // the port in use, which AutoPort resolves
	open    func(string, *serial.Mode) (serial.Port, error) // nil opens the real port; tests replace it
	list    func() ([]string, error)                        // nil lists the USB serial ports; tests replace it

	// Logger, when set, gets a Warn for each failed action the board recovered
	// from, which the run would not show otherwise.
	Logger *slog.Logger
}

// A board that was just reset prints boot noise for about a second and then
// needs a few seconds to find the PC over Bluetooth again, so the handshake
// waits for both before it gives up. Tests shorten the waits.
var (
	bootWait   = 1500 * time.Millisecond // silence or noise this long means it is not the board
	pairWait   = 10 * time.Second        // WAITING this long means it is not paired
	retryEvery = 250 * time.Millisecond
)

// replyWait is how long an action waits for the board's OK, on top of the
// key's hold. The board replies once Bluetooth took the report, which takes
// longer than 300 ms now and then while the radio is busy; the input went out
// all the same, so a short wait stopped runs for nothing.
const replyWait = time.Second

var errNotPaired = errors.New("connect Go Fish ESP32 Probe in Windows Bluetooth settings")

// Name returns the port in use, or "" while closed.
func (b *Board) Name() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.name
}

// usbPorts lists the USB serial ports. The board is one, which leaves out the
// virtual ports Windows makes for Bluetooth devices.
func usbPorts() ([]string, error) {
	details, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, d := range details {
		if d.IsUSB {
			names = append(names, d.Name)
		}
	}
	slices.Sort(names)
	return names, nil
}

// Open checks the firmware and Bluetooth connection without sending any input.
// With AutoPort it asks each USB serial port for the Go Fish firmware, which
// sends one line of text to the ports it tries, and uses the first that answers.
func (b *Board) Open(name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.port != nil {
		return errors.New("ESP32 port is already open")
	}
	if name != AutoPort {
		return b.openPort(name)
	}
	list := b.list
	if list == nil {
		list = usbPorts
	}
	names, err := list()
	if err != nil {
		return fmt.Errorf("list serial ports: %w", err)
	}
	if len(names) == 0 {
		return errors.New("no USB serial port found; connect the ESP32 with its USB cable")
	}
	var tried []string
	for _, candidate := range names {
		err := b.openPort(candidate)
		// A board that answers but is not paired is the right board; stop there.
		if err == nil || errors.Is(err, errNotPaired) {
			return err
		}
		tried = append(tried, err.Error())
	}
	return fmt.Errorf("no Go Fish ESP32 found (%s); flash its firmware and close other programs that use it", strings.Join(tried, "; "))
}

func (b *Board) openPort(name string) error {
	open := b.open
	if open == nil {
		open = serial.Open
	}
	port, err := open(name, &serial.Mode{
		BaudRate: 115200, InitialStatusBits: &serial.ModemOutputBits{},
	})
	if err != nil {
		return fmt.Errorf("open ESP32 on %s: %w", name, err)
	}
	b.port = port
	b.reader = bufio.NewReaderSize(timeoutReader{port}, 64)
	if err := b.handshake(); err != nil {
		b.closePort()
		return fmt.Errorf("ESP32 on %s: %w", name, err)
	}
	b.name = name
	return nil
}

func (b *Board) handshake() error {
	if err := b.port.SetReadTimeout(300 * time.Millisecond); err != nil {
		return err
	}
	start := time.Now()
	ours := false // the Go Fish firmware has answered
	noise := ""   // the last text that was not a Go Fish answer
	for {
		if err := b.port.ResetInputBuffer(); err != nil {
			return err
		}
		b.reader.Reset(timeoutReader{b.port}) // drop boot noise left half read
		reply, err := b.exchange("HELLO")
		waited := time.Since(start)
		switch {
		case err == nil && reply == "GOFISH 1 READY":
			return b.port.SetReadTimeout(replyWait)
		case err == nil && reply == "GOFISH 1 WAITING":
			ours = true
		case !ours && waited >= bootWait:
			if reply != "" {
				noise = reply
			}
			if noise != "" {
				return fmt.Errorf("unexpected firmware response %q", noise)
			}
			return fmt.Errorf("firmware handshake failed: %w", err)
		default:
			if reply != "" {
				noise = reply
			}
		}
		if ours && waited >= pairWait {
			return errNotPaired
		}
		time.Sleep(retryEvery)
	}
}

// PrepareTimedTaps checks for optional hold support without sending input.
// Call it before live actions so a focus check is not followed by negotiation.
func (b *Board) PrepareTimedTaps() error {
	return b.prepareCapability("TAPMS", &b.timed)
}

// PrepareCamera probes for mouse support without moving the pointer.
func (b *Board) PrepareCamera() error {
	return b.prepareCapability("MOUSE", &b.camera)
}

func (b *Board) prepareCapability(command string, supported *bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.port == nil {
		return errors.New("ESP32 port is closed")
	}
	reply, err := b.exchange(command)
	if err != nil {
		b.closePort()
		return fmt.Errorf("ESP32 %s capability: %w", command, err)
	}
	switch reply {
	case command:
		*supported = true
	case "ERR command":
		*supported = false // older firmware
	default:
		b.closePort()
		return fmt.Errorf("ESP32 %s capability: unexpected response %q", command, reply)
	}
	return nil
}

// PrepareButtons probes for the mouse side buttons without sending input.
func (b *Board) PrepareButtons() error {
	return b.prepareCapability("BUTTONS", &b.buttons)
}

// CanPressButtons reports whether the open port supports mouse side buttons.
func (b *Board) CanPressButtons() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buttons
}

// PressButton holds Mouse 4 or Mouse 5 for hold, 40 ms when it is zero, with
// the same failure contract as Tap.
func (b *Board) PressButton(vk byte, hold time.Duration) error {
	if vk != vkMouse4 && vk != vkMouse5 {
		return errors.New("ESP32 presses only Mouse 4 and Mouse 5")
	}
	if hold == 0 {
		hold = 40 * time.Millisecond
	}
	if hold < 10*time.Millisecond || hold > 500*time.Millisecond {
		return errors.New("ESP32 hold must be between 10 and 500 ms")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.port == nil || !b.buttons {
		return errors.New("flash the current ESP32 firmware to press Mouse 4 and Mouse 5")
	}
	if err := b.port.SetReadTimeout(replyWait + hold); err != nil {
		b.closePort()
		return fmt.Errorf("ESP32 button timeout: %w", err)
	}
	return b.sendAction(fmt.Sprintf("BUTTON %d %d", vk-vkMouse4+4, hold.Milliseconds()), "button press")
}

// PreparePointer probes for pointer moves and right clicks without sending input.
func (b *Board) PreparePointer() error {
	return b.prepareCapability("POINTER", &b.pointer)
}

// CanPoint reports whether the open port advertised pointer support.
func (b *Board) CanPoint() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.pointer
}

// ClickRight presses and releases the right mouse button, with the same
// failure contract as Tap.
func (b *Board) ClickRight() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.port == nil || !b.pointer {
		return errors.New("ESP32 pointer support is unavailable")
	}
	return b.sendAction("CLICK", "right click")
}

// maxMoveSteps bounds MoveTo; a path within the game window needs about ten.
const maxMoveSteps = 40

// MoveTo steers the pointer onto target with relative mouse reports. Windows
// scales each report by its pointer speed and acceleration, so every step reads
// the real position from pos and divides the rest of the way by the gain the
// previous step showed.
func (b *Board) MoveTo(target image.Point, pos func() (x, y int, ok bool)) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.port == nil || !b.pointer {
		return errors.New("ESP32 pointer support is unavailable")
	}
	gain := [2]float64{1, 1}
	var trail []string // each step, for the error when the target is not reached
	resynced := false  // a lost reply was already got past with a new HELLO
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
		reply, err := b.exchange(fmt.Sprintf("MOVE %d %d", step[0], step[1]))
		// Getting past a failed step once is safe: the next step reads where
		// the pointer really is, so a report that did go out is not sent twice.
		if !resynced && b.recovered("pointer move", reply, err) {
			resynced = true
			continue
		}
		if err := b.result("pointer move", reply, err); err != nil {
			return err
		}
		sent := time.Now()
		nx, ny := settle(x, y, pos)
		moved := [2]int{nx - x, ny - y}
		trail = append(trail, fmt.Sprintf("%d,%d%+d%+d>%+d%+d/%dms", x, y, step[0], step[1], moved[0], moved[1], time.Since(sent).Milliseconds()))
		for i := range gain {
			if step[i] != 0 && moved[i] != 0 && (moved[i] > 0) == (step[i] > 0) {
				gain[i] = min(max(float64(moved[i])/float64(step[i]), 0.25), 4)
			}
		}
	}
	// Each step reads "from x,y, sent dx dy > moved dx dy / time until Windows applied it".
	return fmt.Errorf("ESP32 pointer did not reach %d,%d after %d steps: %s",
		target.X, target.Y, len(trail), strings.Join(trail[max(0, len(trail)-12):], " "))
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

// reportLag is how long settle waits for a report to move the pointer. The
// firmware acknowledges a report once its Bluetooth stack queued it, and
// Windows applies it a connection interval or more later, past 50 ms while the
// radio is busy (Wi-Fi, interference). A shorter wait sends the next step while
// this one is still on its way: both land, and the pointer overshoots after
// MoveTo already reported it on target.
const reportLag = 250 * time.Millisecond

// settle waits until Windows applies the report that was just acknowledged.
// It returns early once the pointer moves; it waits the whole reportLag only
// for a report that moves it by less than a pixel.
func settle(x, y int, pos func() (int, int, bool)) (int, int) {
	deadline := time.Now().Add(reportLag)
	for {
		nx, ny, ok := pos()
		if ok && (nx != x || ny != y) || time.Now().After(deadline) {
			return nx, ny
		}
		time.Sleep(time.Millisecond)
	}
}

// CanDragCamera reports whether the open port advertised mouse support.
func (b *Board) CanDragCamera() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.camera
}

// DragCamera requests one bounded horizontal drag. The firmware owns the
// press and release. An unacknowledged gesture is never retried.
func (b *Board) DragCamera(dx int) error {
	if dx < -24 || dx > 24 || dx == 0 {
		return errors.New("ESP32 camera drag must be between 1 and 24 counts")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.port == nil || !b.camera {
		return errors.New("ESP32 mouse support is unavailable")
	}
	return b.sendAction(fmt.Sprintf("DRAG %d", dx), "camera drag")
}

// Tap waits for acknowledgment of a bounded press and release. A failed
// transaction closes the port because the press might already have happened.
func (b *Board) Tap(usage byte) error {
	return b.TapFor(usage, 0)
}

// TapFor requests a 10–500 ms hold on current firmware. Zero and legacy
// firmware use the board's default hold. Call PrepareTimedTaps before the
// run to enable the optional command.
func (b *Board) TapFor(usage byte, hold time.Duration) error {
	if usage < 0x04 || usage > 0x65 {
		return errors.New("ESP32 requires a keyboard key")
	}
	if hold != 0 && (hold < 10*time.Millisecond || hold > 500*time.Millisecond) {
		return errors.New("ESP32 hold must be between 10 and 500 ms")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.port == nil {
		return errors.New("ESP32 port is closed")
	}
	command := fmt.Sprintf("TAP %02X", usage)
	custom := hold != 0 && hold != 40*time.Millisecond

	if b.timed && custom {
		command = fmt.Sprintf("%s %d", command, hold.Milliseconds())
	}
	if b.timed {
		if err := b.port.SetReadTimeout(replyWait + hold); err != nil {
			b.closePort()
			return fmt.Errorf("ESP32 tap timeout: %w", err)
		}
	}
	return b.sendAction(command, "tap")
}

// sendAction runs with the port locked. A lost OK or a Bluetooth drop does not
// halt the run once the board answers HELLO again: the action went out or was
// dropped, and a cast or reel that never happened costs only that cast.
func (b *Board) sendAction(command, action string) error {
	reply, err := b.exchange(command)
	if b.recovered(action, reply, err) {
		return nil
	}
	return b.result(action, reply, err)
}

// recovered reports whether the board is ready again after its OK was lost or
// its Bluetooth link dropped. handshake waits up to pairWait for the link.
func (b *Board) recovered(action, reply string, err error) bool {
	cause := ""
	switch {
	case errors.Is(err, os.ErrDeadlineExceeded):
		cause = "reply lost"
	case err == nil && reply == "ERR disconnected":
		cause = "Bluetooth link dropped"
	default:
		return false
	}
	if b.handshake() != nil {
		return false
	}
	if b.Logger != nil {
		b.Logger.Warn("ESP32 recovered", "action", action, "cause", cause)
	}
	return true
}

// result closes the port unless the board acknowledged the action.
func (b *Board) result(action, reply string, err error) error {
	if err == nil && reply == "OK" {
		return nil
	}
	b.closePort()
	if err != nil {
		return fmt.Errorf("ESP32 %s acknowledgment: %w", action, err)
	}
	if reply == "ERR disconnected" {
		return errors.New("the ESP32 lost its Bluetooth link to this PC; reconnect Go Fish ESP32 Probe")
	}
	return fmt.Errorf("ESP32 rejected %s: %q", action, reply)
}

// Close releases the port. The firmware owns the release of any pending tap.
func (b *Board) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closePort()
}

func (b *Board) closePort() {
	if b.port != nil {
		_ = b.port.Close()
		b.port, b.reader, b.name = nil, nil, ""
		b.timed, b.camera, b.pointer, b.buttons = false, false, false, false
	}
}

func (b *Board) exchange(command string) (string, error) {
	data := command + "\n"
	n, err := io.WriteString(b.port, data)
	if err != nil {
		return "", err
	}
	if n != len(data) {
		return "", io.ErrShortWrite
	}
	reply, err := b.reader.ReadSlice('\n')
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
