package esp32

import (
	"errors"
	"strings"
	"testing"

	"go.bug.st/serial"
)

// board is what answers on one fake port: its reply to HELLO, or an open error.
type board struct {
	reply   string
	openErr error
	port    *scriptedPort
}

// autoKeyboard lists the given ports and opens each as its board, noting the order.
func autoKeyboard(order []string, boards map[string]*board, listErr error) (*Keyboard, *[]string) {
	tried := new([]string)
	k := &Keyboard{
		list: func() ([]string, error) { return order, listErr },
		open: func(name string, _ *serial.Mode) (serial.Port, error) {
			*tried = append(*tried, name)
			b := boards[name]
			if b.openErr != nil {
				return nil, b.openErr
			}
			b.port = &scriptedPort{reply: strings.NewReader(b.reply)}
			return b.port, nil
		},
	}
	return k, tried
}

func TestPortNameAcceptsAuto(t *testing.T) {
	for _, in := range []string{"auto", " Auto ", "AUTO"} {
		if got, err := PortName(in); err != nil || got != AutoPort {
			t.Errorf("PortName(%q) = %q, %v; want %q", in, got, err, AutoPort)
		}
	}
}

func TestOpenAutoUsesTheBoardThatAnswers(t *testing.T) {
	boards := map[string]*board{
		"COM3": {openErr: errors.New("access denied")}, // busy in another program
		"COM5": {reply: ""},                            // a USB serial device that stays silent
		"COM7": {reply: "GOFISH 1 READY\n"},
		"COM9": {reply: "GOFISH 1 READY\n"},
	}
	k, tried := autoKeyboard([]string{"COM3", "COM5", "COM7", "COM9"}, boards, nil)
	if err := k.Open(AutoPort); err != nil {
		t.Fatal(err)
	}
	if k.Name() != "COM7" {
		t.Fatalf("Name = %q, want COM7", k.Name())
	}
	if got := strings.Join(*tried, ","); got != "COM3,COM5,COM7" {
		t.Fatalf("tried %s; it must stop at the first board", got)
	}
	if !boards["COM5"].port.closed {
		t.Fatal("the silent port was left open")
	}
	k.Close()
	if k.Name() != "" {
		t.Fatalf("Name = %q after Close, want empty", k.Name())
	}
}

func TestOpenAutoStopsAtABoardThatIsNotPaired(t *testing.T) {
	boards := map[string]*board{
		"COM4": {reply: "GOFISH 1 WAITING\n"},
		"COM8": {reply: "GOFISH 1 READY\n"},
	}
	k, tried := autoKeyboard([]string{"COM4", "COM8"}, boards, nil)
	err := k.Open(AutoPort)
	if err == nil || !strings.Contains(err.Error(), "Bluetooth settings") {
		t.Fatalf("Open returned %v, want the pairing error", err)
	}
	if got := strings.Join(*tried, ","); got != "COM4" {
		t.Fatalf("tried %s; the unpaired board is the right one, so no other port is tried", got)
	}
	if boards["COM4"].port == nil || !boards["COM4"].port.closed {
		t.Fatal("the unpaired board's port was left open")
	}
}

func TestOpenAutoFindsNothing(t *testing.T) {
	cases := []struct {
		name    string
		order   []string
		listErr error
		want    string
	}{
		{"no USB serial ports", nil, nil, "no USB serial port found"},
		{"only other devices", []string{"COM5"}, nil, `no Go Fish ESP32 found (ESP32 on COM5: unexpected firmware response "hello?")`},
		{"listing fails", nil, errors.New("setupapi failed"), "list serial ports"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k, _ := autoKeyboard(tc.order, map[string]*board{"COM5": {reply: "hello?\n"}}, tc.listErr)
			err := k.Open(AutoPort)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Open returned %v, want error containing %q", err, tc.want)
			}
			if k.Name() != "" {
				t.Fatalf("Name = %q after a failed Open", k.Name())
			}
		})
	}
}

func TestOpenNamedPortSkipsDetection(t *testing.T) {
	boards := map[string]*board{"COM3": {reply: "GOFISH 1 READY\n"}}
	k, _ := autoKeyboard(nil, boards, errors.New("must not list"))
	if err := k.Open("COM3"); err != nil {
		t.Fatal(err)
	}
	if k.Name() != "COM3" {
		t.Fatalf("Name = %q, want COM3", k.Name())
	}
}
