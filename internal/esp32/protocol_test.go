package esp32

import (
	"bufio"
	"errors"
	"strings"
	"testing"
	"time"

	"go.bug.st/serial"
)

func bufioReader(p *scriptedPort) *bufio.Reader { return bufio.NewReader(timeoutReader{p}) }

// scriptedPort answers each line the keyboard writes with the next scripted reply.
type scriptedPort struct {
	serial.Port
	reply  *strings.Reader
	writes strings.Builder
	closed bool
}

func init() {
	// A real board needs seconds to boot and reconnect; scripted ones do not.
	bootWait, pairWait, retryEvery = 40*time.Millisecond, 120*time.Millisecond, 5*time.Millisecond
}

// Read returns one scripted line per call, as a board sends one answer per HELLO.
func (p *scriptedPort) Read(b []byte) (int, error) {
	n := 0
	for n < len(b) {
		c, err := p.reply.ReadByte()
		if err != nil {
			if n > 0 {
				return n, nil
			}
			return 0, err
		}
		b[n] = c
		n++
		if c == '\n' {
			break
		}
	}
	return n, nil
}
func (p *scriptedPort) Write(b []byte) (int, error)        { return p.writes.Write(b) }
func (p *scriptedPort) Close() error                       { p.closed = true; return nil }
func (p *scriptedPort) SetReadTimeout(time.Duration) error { return nil }
func (p *scriptedPort) ResetInputBuffer() error            { return nil }

func openWith(port *scriptedPort, openErr error) *Keyboard {
	return &Keyboard{open: func(string, *serial.Mode) (serial.Port, error) {
		if openErr != nil {
			return nil, openErr
		}
		return port, nil
	}}
}

func TestPortName(t *testing.T) {
	for in, want := range map[string]string{"com3": "COM3", " COM12 ": "COM12", "": ""} {
		got, err := PortName(in)
		if err != nil || got != want {
			t.Errorf("PortName(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"3", "COM0", "COMx", "/dev/ttyUSB0"} {
		if _, err := PortName(in); err == nil {
			t.Errorf("PortName(%q) accepted an invalid port", in)
		}
	}
}

func TestSupportsKey(t *testing.T) {
	for _, vk := range []byte{'A', 'Z', '0', '9', 0x70, 0x7b} {
		if !SupportsKey(vk) {
			t.Errorf("SupportsKey(%#x) = false", vk)
		}
	}
	for _, vk := range []byte{0x0d, 0x20, 0x6f, 0x7c} {
		if SupportsKey(vk) {
			t.Errorf("SupportsKey(%#x) = true", vk)
		}
	}
}

func TestOpenHandshake(t *testing.T) {
	cases := []struct {
		name, reply, wantErr string
	}{
		{"ready", "GOFISH 1 READY\n", ""},
		{"waiting for Bluetooth", "GOFISH 1 WAITING\n", "Bluetooth settings"},
		{"unknown firmware", "hello?\n", "unexpected firmware response"},
		{"silent firmware", "", "handshake failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			port := &scriptedPort{reply: strings.NewReader(tc.reply)}
			k := openWith(port, nil)
			err := k.Open("COM3")
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				if port.writes.String() != "HELLO\n" {
					t.Fatalf("writes = %q", port.writes.String())
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Open returned %v, want error containing %q", err, tc.wantErr)
			}
			if !port.closed || k.port != nil {
				t.Fatal("a failed handshake left the port open")
			}
		})
	}
}

func TestOpenErrors(t *testing.T) {
	k := openWith(nil, errors.New("access denied"))
	if err := k.Open("COM3"); err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("Open returned %v, want the serial error", err)
	}

	port := &scriptedPort{reply: strings.NewReader("GOFISH 1 READY\n")}
	k = openWith(port, nil)
	if err := k.Open("COM3"); err != nil {
		t.Fatal(err)
	}
	if err := k.Open("COM3"); err == nil {
		t.Fatal("second Open on an open port succeeded")
	}
}

func TestTapErrorsCloseThePort(t *testing.T) {
	cases := []struct {
		name, reply, wantErr string
	}{
		{"lost Bluetooth", "ERR disconnected\n", "lost its Bluetooth link"},
		{"BLE report failed", "ERR BLE report\n", "rejected tap"},
		{"unknown command", "ERR command\n", "rejected tap"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			port := &scriptedPort{reply: strings.NewReader(tc.reply)}
			k := &Keyboard{port: port, reader: bufioReader(port)}
			err := k.Tap(0x04)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Tap returned %v, want error containing %q", err, tc.wantErr)
			}
			if !port.closed {
				t.Fatal("a rejected tap left the port open")
			}
		})
	}
}

func TestTapRejectsNonKeyboardUsage(t *testing.T) {
	port := &scriptedPort{reply: strings.NewReader("OK\n")}
	k := &Keyboard{port: port, reader: bufioReader(port)}
	for _, usage := range []byte{0x00, 0x03, 0x66, 0xff} {
		if err := k.Tap(usage); err == nil {
			t.Errorf("Tap(%#x) accepted a usage outside the keyboard range", usage)
		}
	}
	if port.writes.Len() != 0 {
		t.Fatalf("invalid usages reached the firmware: %q", port.writes.String())
	}
}

// A board opened by a program that toggles DTR/RTS resets: it prints boot noise,
// then answers WAITING while Bluetooth reconnects, then READY.
func TestOpenWaitsForABoardThatIsBooting(t *testing.T) {
	port := &scriptedPort{reply: strings.NewReader(
		"ets Jul 29 2019 12:21:46\n" + "GOFISH 1 WAITING\n" + "GOFISH 1 WAITING\n" + "GOFISH 1 READY\n")}
	k := openWith(port, nil)
	if err := k.Open("COM3"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(port.writes.String(), "HELLO\n"); got != 4 {
		t.Fatalf("sent HELLO %d times, want 4", got)
	}
}

func TestOpenReportsBootNoiseThatNeverEnds(t *testing.T) {
	port := &scriptedPort{reply: strings.NewReader(strings.Repeat("ets Jul 29 2019 12:21:46\n", 100))}
	err := openWith(port, nil).Open("COM3")
	if err == nil || !strings.Contains(err.Error(), `unexpected firmware response "ets Jul 29 2019 12:21:46"`) {
		t.Fatalf("Open returned %v, want the noise reported", err)
	}
	if strings.Count(port.writes.String(), "HELLO\n") < 2 {
		t.Fatal("gave up after one try, before the board could have finished booting")
	}
}

func TestTapOnClosedKeyboard(t *testing.T) {
	var k Keyboard
	if err := k.Tap(0x04); err == nil {
		t.Fatal("Tap on a closed keyboard succeeded")
	}
}
