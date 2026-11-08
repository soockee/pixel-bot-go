package esp32

import (
	"bufio"
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
