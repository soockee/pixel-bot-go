// Package esp32 sends bounded keyboard taps to the Go Fish BLE firmware.
package esp32

import (
	"bufio"
	"errors"
	"fmt"
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
	case "GOFISH 1 READY":
		return nil
	case "GOFISH 1 WAITING":
		return errors.New("connect Go Fish ESP32 Probe in Windows Bluetooth settings")
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
