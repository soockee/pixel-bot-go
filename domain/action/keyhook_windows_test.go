package action

import (
	"errors"
	"testing"
	"time"
)

// A real key cannot be sent from a test: SendInput keys are injected, which
// CatchKey ignores on purpose. This covers installing and removing the hook.
func TestCatchKeyReportsAMissingKeyAndAFailedPress(t *testing.T) {
	for range 2 { // a second run reuses the one callback
		if _, err := CatchKey(0x77, 50*time.Millisecond, func() error { return nil }); err == nil {
			t.Fatal("no key was sent, want an error")
		}
	}
	boom := errors.New("board gone")
	if _, err := CatchKey(0x77, time.Second, func() error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the press error", err)
	}
}
