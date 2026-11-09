package desktop

// SupportURL is the page where a person can buy the author a coffee.
const SupportURL = "https://buymeacoffee.com/socke"

// supportMilestones are the total catches on this computer at which the
// thank-you card asks once; after the last one it asks every
// supportMilestoneEvery catches. Milestones keep the ask rare and tie it to
// what Go Fish did for the person, not to the calendar.
var supportMilestones = []int{100, 500, 1000, 2500, 5000}

const supportMilestoneEvery = 5000

// Support is the thank-you card in the Fishing view.
type Support struct {
	Ask       bool `json:"ask"`       // show the card
	Milestone int  `json:"milestone"` // the milestone just reached
	Catches   int  `json:"catches"`   // catches on this computer so far
	Seconds   int  `json:"seconds"`   // time Go Fish fished on this computer so far
}

// SupportRun is how a finished run went: its catches, and its reels with no
// fish hooked, which are false bites.
type SupportRun struct {
	Catches, NotHooked int
}

// good reports whether the run went well enough to thank anyone for: at
// least a catch, and no more than one false bite per four catches.
func (r SupportRun) good() bool { return r.Catches > 0 && 4*r.NotHooked <= r.Catches }

// SupportMilestone returns the milestone the card should ask at after a run,
// with total catches on this computer and asked the last milestone it asked
// at, or false. It asks only after a good run; a milestone reached in a bad
// run waits for the next good one. muted means the person said they support
// Go Fish already or not to ask again; a dry run never asks.
func SupportMilestone(total, asked int, run SupportRun, muted, dryRun bool) (int, bool) {
	if muted || dryRun || !run.good() {
		return 0, false
	}
	reached := 0
	for _, m := range supportMilestones {
		if total >= m {
			reached = m
		}
	}
	if last := supportMilestones[len(supportMilestones)-1]; total >= last+supportMilestoneEvery {
		reached = total / supportMilestoneEvery * supportMilestoneEvery
	}
	if reached == 0 || reached <= asked {
		return 0, false
	}
	return reached, true
}

// SetSupport shows or hides the thank-you card.
func (v *View) SetSupport(s Support) { v.state.Support = s }
