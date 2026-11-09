package desktop

import (
	"bufio"
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

// The problem form, .github/ISSUE_TEMPLATE/problem.yml, and the ids of the
// fields the app fills in. GitHub fills a form field from the query parameter
// named after its id.
const (
	issueNewURL   = "https://github.com/soockee/pixel-bot-go/issues/new"
	issueTemplate = "problem.yml"
	issueVersion  = "version"
	issueWindows  = "windows"
	issueDetails  = "details"
)

// detailsLimit keeps the URL under the roughly 8000 characters GitHub and
// browsers accept once the details are escaped.
const detailsLimit = 3000

// ProblemURL returns a new GitHub issue on the problem form, filled in with
// the version, Windows and what the app knows: the run, the setup checks, the
// Settings tab's values and the recent warnings. Nothing is sent until the
// person submits the form in the browser.
func ProblemURL(s State, cfg *config.Config, windows string, warnings []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Run: %s, running %v, casts %d, reels %d, casts without a bite in a row %d\n",
		s.FSM, s.Running, s.Casts, s.Reels, s.FailedInRow)
	fmt.Fprintf(&b, "Status: %s\n", s.Status)
	fmt.Fprintf(&b, "Game window: %q, found %v\n", s.Window, s.GameFound)
	fmt.Fprintf(&b, "Addon: %s (app carries %s)\n", cmp.Or(s.Addon, "game not found"), s.AddonVersion)
	fmt.Fprintf(&b, "Setup done: %v, dry run %v, debug %v, theme %s\n", s.SetupDone, s.DryRun, s.Debug, s.Theme)
	b.WriteString("\nSettings:\n")
	b.WriteString(settingLines(settingsOf(cfg)))
	if len(warnings) > 0 {
		b.WriteString("\nRecent warnings and errors:\n")
		for _, w := range warnings {
			b.WriteString("  " + w + "\n")
		}
	}
	details := b.String()
	if len(details) > detailsLimit {
		details = strings.ToValidUTF8(details[:detailsLimit], "") + "\n(cut short)" // the cut may split a character
	}
	q := url.Values{}
	q.Set("template", issueTemplate)
	q.Set(issueVersion, s.Version)
	q.Set(issueWindows, windows)
	q.Set(issueDetails, details)
	return issueNewURL + "?" + q.Encode()
}

// settingLines lists the settings one "name: value" line each, by name.
func settingLines(s Settings) string {
	data, _ := json.Marshal(s) // a struct of plain fields always marshals
	var values map[string]any
	_ = json.Unmarshal(data, &values)
	var b strings.Builder
	for _, k := range slices.Sorted(maps.Keys(values)) {
		fmt.Fprintf(&b, "  %s: %v\n", k, values[k])
	}
	return b.String()
}

// RecentProblems returns the last n warnings and errors of the JSON log at
// path, oldest first, as "time LEVEL message (error)". A missing or unreadable
// log gives none.
func RecentProblems(path string, n int) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var lines []string
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64*1024), 1024*1024)
	for scan.Scan() {
		var e struct {
			Time  time.Time `json:"time"`
			Level string    `json:"level"`
			Msg   string    `json:"msg"`
			Error string    `json:"error"`
		}
		if json.Unmarshal(scan.Bytes(), &e) != nil || e.Level != "WARN" && e.Level != "ERROR" {
			continue
		}
		line := e.Time.Format("15:04:05") + " " + e.Level + " " + e.Msg
		if e.Error != "" {
			line += " (" + e.Error + ")"
		}
		lines = append(lines, line)
	}
	return lines[max(0, len(lines)-n):]
}
