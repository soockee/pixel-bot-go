package desktop

import "fmt"

// setupSlots is how many steps State carries. State is compared as a value to
// decide what to send, so the steps are an array, not a slice.
const setupSlots = 3

// SetupStep is one thing the bot needs before it can fish.
type SetupStep struct {
	ID     string `json:"id"` // "game", "cast" or "addon"; "" for an unused slot
	Title  string `json:"title"`
	Done   bool   `json:"done"`
	Detail string `json:"detail"` // what to do or what is wrong; a reminder when Done
	Action string `json:"action"` // the label of the button that does it, "" when there is nothing to click
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

	// A current addon already holds the saved Cast key, so there is nothing to choose.
	cast := SetupStep{ID: "cast", Title: "Cast key", Done: s.CastKeyChosen || s.Addon == "current"}
	if cast.Done {
		cast.Detail = "Bobber Marker puts Fishing on this key."
	} else {
		cast.Detail, cast.Action = "Keep F3 or press another spare key. Bobber Marker puts Fishing on it.", "Use this key"
	}

	addon := SetupStep{ID: "addon", Title: "Bobber Marker addon"}
	switch s.Addon {
	case "current":
		addon.Done, addon.Detail = true, "Installed and up to date."
	case "missing":
		addon.Detail, addon.Action = "Install it into the game. It marks your bobber and binds your Cast key.", "Install addon"
	case "outdated":
		addon.Detail, addon.Action = "Update it to apply your keys and the new version.", "Update addon"
	default:
		addon.Detail = "Go Fish needs to find the game folder before it can install Bobber Marker."
	}

	steps[0], steps[1], steps[2] = game, cast, addon

	done = true
	for _, step := range steps {
		done = done && (step.ID == "" || step.Done)
	}
	return steps, done
}
