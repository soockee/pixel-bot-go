package app

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/internal/esp32"
)

// boardOwner holds the one Board. A run, a scheduled key and the board check
// each take it whole until they release it, so they never share the port.
type boardOwner struct {
	mu    sync.Mutex
	board esp32.Board
}

var errBoardBusy = errors.New("the ESP32 is in use by a run; stop fishing first")

// open takes the Board and opens it on port. release closes it and lets the
// next user take it.
func (o *boardOwner) open(port string) (board *esp32.Board, release func(), err error) {
	if !o.mu.TryLock() {
		return nil, nil, errBoardBusy
	}
	if err := o.board.Open(port); err != nil {
		o.mu.Unlock()
		return nil, nil, err
	}
	return &o.board, func() {
		o.board.Close()
		o.mu.Unlock()
	}, nil
}

// portOf is the Board's port in cfg, or why there is none.
func portOf(cfg config.Config) (string, error) {
	if cfg.ESP32Port == "" {
		return "", errors.New("select or connect the ESP32 board in Settings")
	}
	return esp32.PortName(cfg.ESP32Port)
}

// boardNeeds are the Board features a use of it needs from the firmware.
type boardNeeds struct {
	camera      bool // camera drags, for Reposition and idle nudges
	rightClick  bool // pointer moves and right clicks, for the right-click reel
	wander      bool // pointer moves, for idle wanders
	timedTaps   bool // holds other than 40 ms, for a timing profile
	sideButtons bool // Mouse 4 or Mouse 5 is a bound key
}

// needsFor is what a fishing run with run's settings needs from the Board.
func needsFor(run config.Config, timedTaps bool) boardNeeds {
	keys := []string{run.CastKey}
	if run.Schedule || run.Reposition {
		keys = append(keys, run.LogoutKey)
	}
	if run.ReelMethod == config.ReelInteractKey {
		keys = append(keys, run.InteractKey)
	}
	return boardNeeds{
		camera:      run.Reposition || run.IdleMoves,
		rightClick:  run.ReelMethod != config.ReelInteractKey,
		wander:      run.IdleMoves,
		timedTaps:   timedTaps,
		sideButtons: slices.ContainsFunc(keys, isSideButton),
	}
}

// isSideButton reports whether key names Mouse 4 or Mouse 5.
func isSideButton(key string) bool {
	vk, _ := config.KeyCode(key)
	return vk == 0x05 || vk == 0x06
}

// openFor takes the Board, opens it on port and asks it for each feature in
// needs. It refuses, and lets the Board go, when the firmware lacks one.
func (o *boardOwner) openFor(port string, needs boardNeeds) (release func(), err error) {
	board, release, err := o.open(port)
	if err != nil {
		return nil, err
	}
	if err := prepare(board, needs); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

func prepare(board *esp32.Board, needs boardNeeds) error {
	if needs.timedTaps {
		if err := board.PrepareTimedTaps(); err != nil {
			return err
		}
	}
	if needs.camera {
		if err := board.PrepareCamera(); err != nil {
			return err
		}
	}
	if needs.rightClick || needs.wander {
		if err := board.PreparePointer(); err != nil {
			return err
		}
	}
	if err := board.PrepareButtons(); err != nil {
		return err
	}
	switch {
	case needs.sideButtons && !board.CanPressButtons():
		return errors.New("flash the current ESP32 firmware to press Mouse 4 and Mouse 5")
	case needs.rightClick && !board.CanPoint():
		return errors.New("flash the current ESP32 firmware to reel with a right click")
	case needs.wander && !board.CanPoint():
		return errors.New("flash the current ESP32 firmware to move the pointer")
	}
	return nil
}

// openRun opens the Board for a fishing run with run's settings and logs what
// it can do. The run summary counts runs by that log line.
func (o *boardOwner) openRun(run config.Config, timedTaps bool, logger *slog.Logger) (release func(), err error) {
	port, err := portOf(run)
	if err != nil {
		return nil, err
	}
	needs := needsFor(run, timedTaps)
	if release, err = o.openFor(port, needs); err != nil {
		return nil, err
	}
	board := &o.board
	// Only what this run asked the board: a capability it never asked reads false.
	ready := []any{"port", board.Name(), "buttons", board.CanPressButtons()}
	if needs.camera {
		ready = append(ready, "camera", board.CanDragCamera())
	}
	if needs.rightClick || needs.wander {
		ready = append(ready, "pointer", board.CanPoint())
	}
	logger.Info("ESP32 keyboard ready", ready...)
	return release, nil
}

// find opens the ESP32 on port, or finds it with esp32.AutoPort, which checks
// the firmware and its Bluetooth link without sending input, then closes it
// again and returns the port that answered.
func (o *boardOwner) find(port string) (string, error) {
	board, release, err := o.open(port)
	if err != nil {
		return "", err
	}
	defer release()
	return board.Name(), nil
}

const (
	pings     = 20
	testVK    = 0x77 // F8: swallowed by the check, so no window gets it
	testUsage = 0x41 // F8 as a HID usage, the same on every keyboard layout
)

// test checks the whole way input takes: the USB serial link (PING and
// PONG), a key from the board through Bluetooth into Windows, caught and
// swallowed before any window sees it, and on pointer firmware a one-count
// pointer move and back. It returns a line for the person, or what failed.
func (o *boardOwner) test(port string) (string, error) {
	board, release, err := o.open(port)
	if err != nil {
		return "", err
	}
	defer release()

	ping, err := board.Ping(pings)
	switch {
	case err != nil:
		return "", err
	case ping.Answered < ping.Sent:
		return "", fmt.Errorf("the board on %s answered %d of %d checks over USB. Check the USB cable, or try another USB port", board.Name(), ping.Answered, ping.Sent)
	case !ping.Connected:
		return "", fmt.Errorf("the board on %s answers over USB but has no Bluetooth link. Pair Go Fish ESP32 Probe in Windows Bluetooth settings", board.Name())
	}
	line := fmt.Sprintf("Board on %s: %d of %d answers over USB, typically %s.", board.Name(), ping.Answered, ping.Sent, ms(ping.Typical))

	key, err := action.CatchKey(testVK, 2*time.Second, func() error { return board.Tap(testUsage) })
	if err != nil {
		return "", errors.New(line + " But its key does not reach Windows over Bluetooth. Remove Go Fish ESP32 Probe in Windows Bluetooth settings and pair it again")
	}
	line += fmt.Sprintf(" A key reaches Windows over Bluetooth in %s.", ms(key))

	if err := board.PreparePointer(); err != nil || !board.CanPoint() {
		return line + " This firmware has no pointer, so reel with the Interact key or flash the current firmware.", nil
	}
	pointer, err := board.Nudge(action.CursorPos)
	if err != nil {
		return "", errors.New(line + " But its pointer moves do not reach Windows. Remove Go Fish ESP32 Probe in Windows Bluetooth settings and pair it again")
	}
	return line + fmt.Sprintf(" The pointer reaches Windows in %s.", ms(pointer)), nil
}

// ms writes d in whole milliseconds, at least 1.
func ms(d time.Duration) string {
	return fmt.Sprintf("%d ms", max(d.Milliseconds(), 1))
}
