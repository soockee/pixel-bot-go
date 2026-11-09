package desktop

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/soocke/pixel-bot-go/config"
)

// The app fills in the problem form by its field ids; renaming one in the
// form would silently leave that field empty.
func TestProblemFormHasTheFieldsTheAppFills(t *testing.T) {
	form, err := os.ReadFile(filepath.Join("..", "..", ".github", "ISSUE_TEMPLATE", issueTemplate))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{issueVersion, issueWindows, issueDetails} {
		if !strings.Contains(string(form), "id: "+id+"\n") {
			t.Errorf("%s has no field with id %q", issueTemplate, id)
		}
	}
}

func TestProblemURLFillsTheForm(t *testing.T) {
	s := State{FSM: "monitoring", Running: true, Window: "World of Warcraft", GameFound: true, Addon: "current", Version: "v1.2.0"}
	cfg := config.DefaultConfig()
	raw := ProblemURL(s, cfg, "Windows 11 (build 26100)", []string{"10:00:00 ERROR sound capture failed (access denied)"})
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Host != "github.com" || u.Path != "/soockee/pixel-bot-go/issues/new" || q.Get("template") != issueTemplate {
		t.Fatalf("url = %s", raw)
	}
	if q.Get(issueVersion) != "v1.2.0" || q.Get(issueWindows) != "Windows 11 (build 26100)" {
		t.Errorf("version %q, windows %q", q.Get(issueVersion), q.Get(issueWindows))
	}
	details := q.Get(issueDetails)
	for _, want := range []string{"Run: monitoring, running true", `"World of Warcraft"`, "biteDetector: splash_sound", "castKey: " + cfg.CastKey, "sound capture failed (access denied)"} {
		if !strings.Contains(details, want) {
			t.Errorf("details lack %q:\n%s", want, details)
		}
	}
	if len(raw) > 8000 {
		t.Errorf("url is %d characters, want at most 8000", len(raw))
	}
}

func TestProblemURLStaysShortWithLongWarnings(t *testing.T) {
	long := []string{strings.Repeat("é", 2000), strings.Repeat("x", 2000)}
	raw := ProblemURL(State{}, config.DefaultConfig(), "", long)
	u, _ := url.Parse(raw)
	details := u.Query().Get(issueDetails)
	if !strings.HasSuffix(details, "(cut short)") || len(raw) > 8000 {
		t.Errorf("details %d bytes, url %d characters", len(details), len(raw))
	}
}

func TestRecentProblemsKeepsTheLastWarningsAndErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.json")
	log := `{"time":"2026-11-09T14:00:01+01:00","level":"INFO","msg":"fishing state transition"}
{"time":"2026-11-09T14:00:02+01:00","level":"WARN","msg":"first"}
not json
{"time":"2026-11-09T14:00:03+01:00","level":"ERROR","msg":"second","error":"boom"}
{"time":"2026-11-09T14:00:04+01:00","level":"WARN","msg":"third"}
`
	if err := os.WriteFile(path, []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	got := RecentProblems(path, 2)
	want := []string{"14:00:03 ERROR second (boom)", "14:00:04 WARN third"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", got, want)
	}
	if RecentProblems(filepath.Join(t.TempDir(), "missing"), 5) != nil {
		t.Error("a missing log gave problems")
	}
}
