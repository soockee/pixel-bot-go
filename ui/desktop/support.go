package desktop

import "time"

// SupportURL is the page where a person can buy the author a coffee.
const SupportURL = "https://buymeacoffee.com/socke"

// The thank-you card shows after a run that clearly saved time, never twice
// in a short while, and leaves people alone longer once they opened the page.
const (
	supportRunCatches   = 10                  // catches in the run that just ended
	supportTotalCatches = 20                  // catches on this computer so far
	supportAskEvery     = 14 * 24 * time.Hour // after "Not now"
	supportAfterOpen    = 90 * 24 * time.Hour // after opening the coffee page
)

// Support is the thank-you card in the Fishing view.
type Support struct {
	Ask        bool `json:"ask"`        // show the card
	RunCatches int  `json:"runCatches"` // catches in the run that just ended
	Catches    int  `json:"catches"`    // catches on this computer so far
}

// AskForSupport reports whether a run that ended at now with runCatches
// catches, of total on this computer, earns the thank-you card. askedAt and
// openedAt are when the card last showed and when the coffee page was last
// opened, zero for never. A dry run never asks.
func AskForSupport(runCatches, total int, askedAt, openedAt, now time.Time, dryRun bool) bool {
	return !dryRun &&
		runCatches >= supportRunCatches && total >= supportTotalCatches &&
		(askedAt.IsZero() || now.Sub(askedAt) >= supportAskEvery) &&
		(openedAt.IsZero() || now.Sub(openedAt) >= supportAfterOpen)
}

// SetSupport shows or hides the thank-you card with the run's and the total
// catches.
func (v *View) SetSupport(s Support) { v.state.Support = s }
