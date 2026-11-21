package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/internal/esp32"
)

// findBoard opens the ESP32 on port, or finds it with esp32.AutoPort, which
// checks the firmware and its Bluetooth link without sending input, then
// closes it again and returns the port that answered.
func findBoard(port string) (string, error) {
	var keyboard esp32.Keyboard
	if err := keyboard.Open(port); err != nil {
		return "", err
	}
	defer keyboard.Close()
	return keyboard.Name(), nil
}

const (
	pings     = 20
	testVK    = 0x77 // F8: swallowed by the check, so no window gets it
	testUsage = 0x41 // F8 as a HID usage, the same on every keyboard layout
)

// testBoard checks the whole way input takes: the USB serial link (PING and
// PONG), a key from the board through Bluetooth into Windows, caught and
// swallowed before any window sees it, and on pointer firmware a one-count
// pointer move and back. It returns a line for the person, or what failed.
func testBoard(port string) (string, error) {
	var keyboard esp32.Keyboard
	if err := keyboard.Open(port); err != nil {
		return "", err
	}
	defer keyboard.Close()

	ping, err := keyboard.Ping(pings)
	switch {
	case err != nil:
		return "", err
	case ping.Answered < ping.Sent:
		return "", fmt.Errorf("the board on %s answered %d of %d checks over USB. Check the USB cable, or try another USB port", keyboard.Name(), ping.Answered, ping.Sent)
	case !ping.Connected:
		return "", fmt.Errorf("the board on %s answers over USB but has no Bluetooth link. Pair Go Fish ESP32 Probe in Windows Bluetooth settings", keyboard.Name())
	}
	line := fmt.Sprintf("Board on %s: %d of %d answers over USB, typically %s.", keyboard.Name(), ping.Answered, ping.Sent, ms(ping.Typical))

	key, err := action.CatchKey(testVK, 2*time.Second, func() error { return keyboard.Tap(testUsage) })
	if err != nil {
		return "", errors.New(line + " But its key does not reach Windows over Bluetooth. Remove Go Fish ESP32 Probe in Windows Bluetooth settings and pair it again")
	}
	line += fmt.Sprintf(" A key reaches Windows over Bluetooth in %s.", ms(key))

	if err := keyboard.PreparePointer(); err != nil || !keyboard.CanPoint() {
		return line + " This firmware has no pointer, so reel with the Interact key or flash the current firmware.", nil
	}
	pointer, err := keyboard.Nudge(action.CursorPos)
	if err != nil {
		return "", errors.New(line + " But its pointer moves do not reach Windows. Remove Go Fish ESP32 Probe in Windows Bluetooth settings and pair it again")
	}
	return line + fmt.Sprintf(" The pointer reaches Windows in %s.", ms(pointer)), nil
}

// ms writes d in whole milliseconds, at least 1.
func ms(d time.Duration) string {
	return fmt.Sprintf("%d ms", max(d.Milliseconds(), 1))
}
