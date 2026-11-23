package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Windows virtual-key codes of the mouse side buttons.
const (
	vkMouse4 = 0x05
	vkMouse5 = 0x06
)

// KeyCode converts a key name (F1 to F12, a letter, a digit, Mouse4 or Mouse5)
// into a Windows virtual-key code. It reports false for any other name.
func KeyCode(key string) (byte, bool) {
	k := strings.ToUpper(strings.TrimSpace(key))
	switch k {
	case "MOUSE4":
		return vkMouse4, true
	case "MOUSE5":
		return vkMouse5, true
	}
	if len(k) == 1 && (k[0] >= 'A' && k[0] <= 'Z' || k[0] >= '0' && k[0] <= '9') {
		return k[0], true // letters and top-row digits match their VK codes
	}
	if num, ok := strings.CutPrefix(k, "F"); ok {
		if n, err := strconv.ParseUint(num, 10, 8); err == nil && n >= 1 && n <= 12 {
			return byte(0x70 + n - 1), true // VK_F1 = 0x70
		}
	}
	return 0, false
}

// CheckKeys returns the first rule the key bindings break. A key clashes only
// with keys a run presses: Interact with the Interact reel, Logout once it is
// set, and W, A, S and D while repositioning. Logout may still be empty here;
// CheckRunKeys asks for it.
func (c Config) CheckKeys() error {
	cast, ok := KeyCode(c.CastKey)
	if !ok {
		return fmt.Errorf("%q is not a Cast key Go Fish can press", c.CastKey)
	}
	used := map[byte]string{cast: "Cast"}
	interact, interactOK := KeyCode(c.InteractKey)
	switch {
	case c.ReelMethod == ReelInteractKey && !interactOK:
		return errors.New("set the Interact key before enabling this reel method")
	case c.ReelMethod == ReelInteractKey && interact == cast:
		return errors.New("the Interact key must differ from the Cast key")
	case c.ReelMethod == ReelInteractKey:
		used[interact] = "Interact"
	case strings.TrimSpace(c.InteractKey) != "" && !interactOK:
		return fmt.Errorf("%q is not an Interact key Go Fish can press", c.InteractKey)
	}
	if strings.TrimSpace(c.LogoutKey) != "" {
		logout, ok := KeyCode(c.LogoutKey)
		if !ok {
			return fmt.Errorf("%q is not a Logout key Go Fish can press", c.LogoutKey)
		}
		if other, clash := used[logout]; clash {
			return fmt.Errorf("the Logout key must differ from the %s key", other)
		}
		used[logout] = "Logout"
	}
	if c.Reposition {
		for _, key := range "WASD" {
			vk, _ := KeyCode(string(key))
			if other, clash := used[vk]; clash {
				return fmt.Errorf("repositioning walks with %c, which is assigned to %s", key, other)
			}
		}
	}
	return nil
}

// CheckRunKeys is CheckKeys for starting a run, which also needs the Logout
// key when breaks or repositioning may log out.
func (c Config) CheckRunKeys() error {
	if (c.Schedule || c.Reposition) && strings.TrimSpace(c.LogoutKey) == "" {
		return errors.New("set the Logout key: breaks and reposition log out")
	}
	return c.CheckKeys()
}
