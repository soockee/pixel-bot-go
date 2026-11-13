// Package esp32 sends bounded keyboard taps, camera drags, pointer moves and
// right clicks to the Go Fish BLE firmware.
package esp32

import (
	"bufio"
	"errors"
	"fmt"
	"image"
	"io"
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

// Keyboard owns one port for the duration of a fishing run.
type Keyboard struct {
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
}

// A board that was just reset prints boot noise for about a second and then
// needs a few seconds to find the PC over Bluetooth again, so the handshake
// waits for both before it gives up. Tests shorten the waits.
var (
	bootWait   = 1500 * time.Millisecond // silence or noise this long means it is not the board
	pairWait   = 10 * time.Second        // WAITING this long means it is not paired
	retryEvery = 250 * time.Millisecond
)

var errNotPaired = errors.New("connect Go Fish ESP32 Probe in Windows Bluetooth settings")

// Name returns the port in use, or "" while closed.
func (k *Keyboard) Name() string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.name
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
func (k *Keyboard) Open(name string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.port != nil {
		return errors.New("ESP32 port is already open")
	}
	if name != AutoPort {
		return k.openPort(name)
	}
	list := k.list
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
		err := k.openPort(candidate)
		// A board that answers but is not paired is the right board; stop there.
		if err == nil || errors.Is(err, errNotPaired) {
			return err
		}
		tried = append(tried, err.Error())
	}
	return fmt.Errorf("no Go Fish ESP32 found (%s); flash its firmware and close other programs that use it", strings.Join(tried, "; "))
}

func (k *Keyboard) openPort(name string) error {
	open := k.open
	if open == nil {
		open = serial.Open
	}
	port, err := open(name, &serial.Mode{
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
	k.name = name
	return nil
}

func (k *Keyboard) handshake() error {
	if err := k.port.SetReadTimeout(300 * time.Millisecond); err != nil {
		return err
	}
	start := time.Now()
	ours := false // the Go Fish firmware has answered
	noise := ""   // the last text that was not a Go Fish answer
	for {
		if err := k.port.ResetInputBuffer(); err != nil {
			return err
		}
		k.reader.Reset(timeoutReader{k.port}) // drop boot noise left half read
		reply, err := k.exchange("HELLO")
		waited := time.Since(start)
		switch {
		case err == nil && reply == "GOFISH 1 READY":
			return nil
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
func (k *Keyboard) PrepareTimedTaps() error {
	return k.prepareCapability("TAPMS", &k.timed)
}

// PrepareCamera probes for mouse support without moving the pointer.
func (k *Keyboard) PrepareCamera() error {
	return k.prepareCapability("MOUSE", &k.camera)
}

func (k *Keyboard) prepareCapability(command string, supported *bool) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.port == nil {
		return errors.New("ESP32 port is closed")
	}
	reply, err := k.exchange(command)
	if err != nil {
		k.closePort()
		return fmt.Errorf("ESP32 %s capability: %w", command, err)
	}
	switch reply {
	case command:
		*supported = true
	case "ERR command":
		*supported = false // older firmware
	default:
		k.closePort()
		return fmt.Errorf("ESP32 %s capability: unexpected response %q", command, reply)
	}
	return nil
}

// PrepareButtons probes for the mouse side buttons without sending input.
func (k *Keyboard) PrepareButtons() error {
	return k.prepareCapability("BUTTONS", &k.buttons)
}

// CanPressButtons reports whether the open port supports mouse side buttons.
func (k *Keyboard) CanPressButtons() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.buttons
}

// PressButton holds Mouse 4 or Mouse 5 for hold, 40 ms when it is zero, with
// the same failure contract as Tap.
func (k *Keyboard) PressButton(vk byte, hold time.Duration) error {
	if vk != vkMouse4 && vk != vkMouse5 {
		return errors.New("ESP32 presses only Mouse 4 and Mouse 5")
	}
	if hold == 0 {
		hold = 40 * time.Millisecond
	}
	if hold < 10*time.Millisecond || hold > 500*time.Millisecond {
		return errors.New("ESP32 hold must be between 10 and 500 ms")
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.port == nil || !k.buttons {
		return errors.New("flash the current ESP32 firmware to press Mouse 4 and Mouse 5")
	}
	if err := k.port.SetReadTimeout(300*time.Millisecond + hold); err != nil {
		k.closePort()
		return fmt.Errorf("ESP32 button timeout: %w", err)
	}
	return k.sendAction(fmt.Sprintf("BUTTON %d %d", vk-vkMouse4+4, hold.Milliseconds()), "button press")
}

// PreparePointer probes for pointer moves and right clicks without sending input.
func (k *Keyboard) PreparePointer() error {
	return k.prepareCapability("POINTER", &k.pointer)
}

// CanPoint reports whether the open port advertised pointer support.
func (k *Keyboard) CanPoint() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.pointer
}

// ClickRight presses and releases the right mouse button, with the same
// failure contract as Tap.
func (k *Keyboard) ClickRight() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.port == nil || !k.pointer {
		return errors.New("ESP32 pointer support is unavailable")
	}
	return k.sendAction("CLICK", "right click")
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
	if k.port == nil || !k.pointer {
		return errors.New("ESP32 pointer support is unavailable")
	}
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
		if err := k.sendAction(fmt.Sprintf("MOVE %d %d", step[0], step[1]), "pointer move"); err != nil {
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

// CanDragCamera reports whether the open port advertised mouse support.
func (k *Keyboard) CanDragCamera() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.camera
}

// DragCamera requests one bounded horizontal drag. The firmware owns the
// press and release. An unacknowledged gesture is never retried.
func (k *Keyboard) DragCamera(dx int) error {
	if dx < -24 || dx > 24 || dx == 0 {
		return errors.New("ESP32 camera drag must be between 1 and 24 counts")
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.port == nil || !k.camera {
		return errors.New("ESP32 mouse support is unavailable")
	}
	return k.sendAction(fmt.Sprintf("DRAG %d", dx), "camera drag")
}

// Tap waits for acknowledgment of a bounded press and release. A failed
// transaction closes the port because the press might already have happened.
func (k *Keyboard) Tap(usage byte) error {
	return k.TapFor(usage, 0)
}

// TapFor requests a 10–500 ms hold on current firmware. Zero and legacy
// firmware use the board's default hold. Call PrepareTimedTaps before the
// run to enable the optional command.
func (k *Keyboard) TapFor(usage byte, hold time.Duration) error {
	if usage < 0x04 || usage > 0x65 {
		return errors.New("ESP32 requires a keyboard key")
	}
	if hold != 0 && (hold < 10*time.Millisecond || hold > 500*time.Millisecond) {
		return errors.New("ESP32 hold must be between 10 and 500 ms")
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.port == nil {
		return errors.New("ESP32 port is closed")
	}
	command := fmt.Sprintf("TAP %02X", usage)
	custom := hold != 0 && hold != 40*time.Millisecond

	if k.timed && custom {
		command = fmt.Sprintf("%s %d", command, hold.Milliseconds())
	}
	if k.timed {
		if err := k.port.SetReadTimeout(300*time.Millisecond + hold); err != nil {
			k.closePort()
			return fmt.Errorf("ESP32 tap timeout: %w", err)
		}
	}
	return k.sendAction(command, "tap")
}

// sendAction runs with the port locked.
func (k *Keyboard) sendAction(command, action string) error {
	reply, err := k.exchange(command)
	if err == nil && reply == "OK" {
		return nil
	}
	k.closePort()
	if err != nil {
		return fmt.Errorf("ESP32 %s acknowledgment: %w", action, err)
	}
	if reply == "ERR disconnected" {
		return errors.New("the ESP32 lost its Bluetooth link to this PC; reconnect Go Fish ESP32 Probe")
	}
	return fmt.Errorf("ESP32 rejected %s: %q", action, reply)
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
		k.port, k.reader, k.name = nil, nil, ""
		k.timed, k.camera, k.pointer, k.buttons = false, false, false, false
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
