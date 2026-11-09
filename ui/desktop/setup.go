package desktop

import "fmt"

// setupSlots is how many steps State carries. State is compared as a value to
// decide what to send, so the steps are an array, not a slice.
const setupSlots = 2

// SetupStep is one thing the bot needs before it can fish.
type SetupStep struct {
	ID     string `json:"id"` // "game" or "addon"; "" for an unused slot
	Title  string `json:"title"`
	Done   bool   `json:"done"`
	Detail string `json:"detail"` // what to do or what is wrong; a reminder when Done
	Action string `json:"action"` // the label of the button that does it, "" when there is nothing to click
}

// setupSteps decides what is still missing from what State already knows.
// A dry run needs none of it: it sends no input, needs no account, and
// usually targets the fake game.
func setupSteps(s State) (steps [setupSlots]SetupStep, done bool) {
	if s.DryRun {
		return steps, true
	}
	game := SetupStep{ID: "game", Title: "Game window", Done: s.GameFound}
	if !game.Done {
		switch s.Window {
		case "", "<none>", defaultWindow:
			game.Detail = "Start World of Warcraft: Forever. Go Fish finds its window by itself."
		default:
			game.Detail = fmt.Sprintf("The window %q is not open. Start it, or choose another under Game window.", s.Window)
		}
	}

	addon := SetupStep{ID: "addon", Title: "Bobber Marker addon"}
	switch s.Addon {
	case "current":
		addon.Done, addon.Detail = true, "Type /reload in the game after an install or update."
	case "missing":
		addon.Detail, addon.Action = "Install it into the game. It marks your bobber and sets up your Cast key.", "Install"
	case "outdated":
		addon.Detail, addon.Action = "Update it for the new version or your changed keys, then type /reload in the game.", "Update"
	default:
		addon.Detail = "Start the game, so Go Fish can find its folder."
	}

	steps[0], steps[1] = game, addon

	done = true
	for _, step := range steps {
		done = done && (step.ID == "" || step.Done)
	}
	return steps, done
}
