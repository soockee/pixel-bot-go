package desktop

import (
	"fmt"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/internal/esp32"
)

// setupSlots is how many steps State carries. State is compared as a value to
// decide what to send, so the steps are an array, not a slice.
const setupSlots = 4

// SetupStep is one thing the bot needs before it can fish.
type SetupStep struct {
	ID     string `json:"id"` // "game", "board", "keys" or "addon"; "" for an unused slot
	Title  string `json:"title"`
	Done   bool   `json:"done"`
	Detail string `json:"detail"` // what to do or what is wrong; a reminder when Done
	Action string `json:"action"` // the label of the button that does it, "" when there is nothing to click
	// Fields are the settings shown with the step while it is the current one,
	// by their Settings JSON names; "" for an unused slot.
	Fields [4]string `json:"fields"`
}

// setupSteps decides what is still missing from what State already knows.
// A dry run needs none of it: it sends no input and
// usually targets the fake game.
func setupSteps(s State) (steps [setupSlots]SetupStep, done bool) {
	if s.DryRun {
		return steps, true
	}
	game := SetupStep{ID: "game", Title: "Game window", Done: s.GameFound}
	if game.Done {
		game.Detail = fmt.Sprintf("Found the %s window.", s.Window)
	} else {
		switch s.Window {
		case "", "<none>", defaultWindow:
			game.Detail = "Waiting for the WoW window. Open WoW: Forever, or select its window if it is already open."
		default:
			game.Detail = fmt.Sprintf("The window %q is not open. Start it, or choose another under Game window.", s.Window)
		}
	}

	board := SetupStep{ID: "board", Title: "ESP32 board", Done: s.ESP32Port != "", Fields: [4]string{"esp32Port"}}
	switch {
	case s.BoardFound != "":
		board.Detail = fmt.Sprintf("Found the board on %s.", s.BoardFound)
	case s.ESP32Port == esp32.AutoPort:
		board.Detail = "Go Fish finds the board on its USB cable when fishing starts."
	case board.Done:
		board.Detail = fmt.Sprintf("Uses the board on %s.", s.ESP32Port)
	default:
		board.Detail, board.Action = "Plug the Go Fish ESP32 into this PC with its USB cable and pair it over Bluetooth. Then find it, or choose its COM port.", "Find the board"
	}

	// A current addon already holds the saved keys, so there is nothing to choose.
	keys := SetupStep{ID: "keys", Title: "Game keys", Done: s.KeysChosen || s.Addon == "current", Fields: s.Keys}
	if keys.Done {
		keys.Detail = "Bobber Marker binds these keys in WoW."
	} else {
		keys.Detail, keys.Action = "F9 to F12 are suggested: WoW leaves them free. Keep them or press other spare keys. Bobber Marker binds them in WoW.", "Use these keys"
	}

	addon := SetupStep{ID: "addon", Title: "Bobber Marker addon"}
	switch s.Addon {
	case "current":
		addon.Done, addon.Detail = true, "Installed and up to date."
	case "missing":
		addon.Detail, addon.Action = "Install it into the game. It marks your bobber and binds your keys.", "Install addon"
	case "outdated":
		addon.Detail, addon.Action = "Update it to apply your keys and the new version.", "Update addon"
	default:
		addon.Detail = "Go Fish needs to find the game folder before it can install Bobber Marker."
	}

	steps = [setupSlots]SetupStep{game, board, keys, addon}
	done = true
	for _, step := range steps {
		done = done && (step.ID == "" || step.Done)
	}
	return steps, done
}

// setupKeys are the key settings the next run uses, the same set the addon
// binds (app.addonKeys), plus the optional Target passing player key.
func setupKeys(c *config.Config) (keys [4]string) {
	n := 0
	add := func(name string) { keys[n], n = name, n+1 }
	add("castKey")
	if c.ReelMethod == config.ReelInteractKey {
		add("interactKey")
	}
	add("targetPlayerKey")
	if c.Schedule || c.Reposition {
		add("logoutKey")
	}
	return keys
}
