// Package esp32 sends bounded keyboard taps to the Go Fish BLE firmware.
package esp32

import (
	"bufio"
	"errors"
	"fmt"
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

// SupportsKey reports whether the backend accepts this Windows virtual key.
func SupportsKey(vk byte) bool {
	return vk >= 'A' && vk <= 'Z' || vk >= '0' && vk <= '9' || vk >= 0x70 && vk <= 0x7b
}

// Keyboard owns one port for the duration of a fishing run.
type Keyboard struct {
	mu     sync.Mutex
	port   serial.Port
	reader *bufio.Reader
	name   string                                          // the port in use, which AutoPort resolves
	open   func(string, *serial.Mode) (serial.Port, error) // nil opens the real port; tests replace it
	list   func() ([]string, error)                        // nil lists the USB serial ports; tests replace it
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

// Tap waits for acknowledgment of a bounded press and release. A failed
// transaction closes the port because the press might already have happened.
func (k *Keyboard) Tap(usage byte) error {
	if usage < 0x04 || usage > 0x65 {
		return errors.New("ESP32 requires a keyboard key")
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.port == nil {
		return errors.New("ESP32 port is closed")
	}
	reply, err := k.exchange(fmt.Sprintf("TAP %02X", usage))
	if err == nil && reply == "OK" {
		return nil
	}
	k.closePort()
	if err != nil {
		return fmt.Errorf("ESP32 tap acknowledgment: %w", err)
	}
	if reply == "ERR disconnected" {
		return errors.New("the ESP32 lost its Bluetooth link to this PC; reconnect Go Fish ESP32 Probe")
	}
	return fmt.Errorf("ESP32 rejected tap: %q", reply)
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
