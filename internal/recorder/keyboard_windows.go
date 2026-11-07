package recorder

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"github.com/soocke/pixel-bot-go/internal/playtrace"
)

type keyboardData struct {
	VK, Scan, Flags, Time uint32
	ExtraInfo             uintptr
}

type keyEvent = playtrace.Key

type keyboardTracker struct {
	pressed [256]bool
}

func (s *keyboardTracker) clear() { clear(s.pressed[:]) }

func (s *keyboardTracker) event(data keyboardData, down bool, now int64, segment int) inputEvent {
	key := keyEvent{Name: keyName(data.VK), VK: data.VK, Scan: data.Scan, Extended: data.Flags&1 != 0}
	kind := "key_up"
	if down {
		kind = "key_down"
	}
	if data.VK < uint32(len(s.pressed)) {
		key.Repeat = down && s.pressed[data.VK]
		s.pressed[data.VK] = down
	}
	if down && !isModifier(data.VK) {
		var parts []string
		for _, modifier := range []struct {
			name string
			keys []uint32
		}{
			{"Ctrl", []uint32{0x11, 0xA2, 0xA3}}, {"Alt", []uint32{0x12, 0xA4, 0xA5}},
			{"Shift", []uint32{0x10, 0xA0, 0xA1}}, {"Win", []uint32{0x5B, 0x5C}},
		} {
			for _, vk := range modifier.keys {
				if s.pressed[vk] {
					parts = append(parts, modifier.name)
					break
				}
			}
		}
		if len(parts) != 0 {
			key.Chord = strings.Join(append(parts, key.Name), "+")
		}
	}
	return inputEvent{Type: kind, ElapsedUS: now, Segment: segment, Key: &key}
}

func isModifier(vk uint32) bool {
	return vk >= 0x10 && vk <= 0x12 || vk >= 0xA0 && vk <= 0xA5 || vk == 0x5B || vk == 0x5C
}

var keyNames = map[uint32]string{
	0x08: "Backspace", 0x09: "Tab", 0x0D: "Enter", 0x10: "Shift", 0x11: "Ctrl", 0x12: "Alt",
	0x13: "Pause", 0x14: "Caps Lock", 0x1B: "Esc", 0x20: "Space",
	0x21: "Page Up", 0x22: "Page Down", 0x23: "End", 0x24: "Home",
	0x25: "Left", 0x26: "Up", 0x27: "Right", 0x28: "Down",
	0x2C: "Print Screen", 0x2D: "Insert", 0x2E: "Delete",
	0x5B: "Left Win", 0x5C: "Right Win", 0x5D: "Menu",
	0x6A: "Numpad *", 0x6B: "Numpad +", 0x6D: "Numpad -", 0x6E: "Numpad .", 0x6F: "Numpad /",
	0x90: "Num Lock", 0x91: "Scroll Lock",
	0xA0: "Left Shift", 0xA1: "Right Shift", 0xA2: "Left Ctrl", 0xA3: "Right Ctrl", 0xA4: "Left Alt", 0xA5: "Right Alt",
}

func keyName(vk uint32) string {
	switch {
	case vk >= 'A' && vk <= 'Z' || vk >= '0' && vk <= '9':
		return string(rune(vk))
	case vk >= 0x70 && vk <= 0x87:
		return fmt.Sprintf("F%d", vk-0x6F)
	case vk >= 0x60 && vk <= 0x69:
		return fmt.Sprintf("Numpad %d", vk-0x60)
	}
	if name := keyNames[vk]; name != "" {
		return name
	}
	return fmt.Sprintf("VK_%02X", vk)
}

func openKeyboardHook(observe func(keyboardData, bool)) (func(), error) {
	callback := syscall.NewCallback(func(code, msg uintptr, data *keyboardData) uintptr {
		const injected = 0x10
		if code == 0 && data != nil && data.Flags&injected == 0 {
			switch msg {
			case 0x0100, 0x0104: // WM_KEYDOWN, WM_SYSKEYDOWN
				observe(*data, true)
			case 0x0101, 0x0105: // WM_KEYUP, WM_SYSKEYUP
				observe(*data, false)
			}
		}
		next, _, _ := callNextHook.Call(0, code, msg, uintptr(unsafe.Pointer(data)))
		return next
	})
	module, _, err := getModuleHandle.Call(0)
	if module == 0 {
		return nil, fmt.Errorf("find recorder module: %w", err)
	}
	hook, _, err := setWindowsHook.Call(13, callback, module, 0) // WH_KEYBOARD_LL
	if hook == 0 {
		return nil, fmt.Errorf("install keyboard recorder: %w", err)
	}
	return func() { unhookWindowsHook.Call(hook) }, nil
}
