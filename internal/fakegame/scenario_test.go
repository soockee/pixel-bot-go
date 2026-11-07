package fakegame

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadScenario(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	sc, err := LoadScenario(write("ok.json", `{"loop":false,"timeline":[{"phase":"bobber","ms":100}]}`))
	if err != nil || sc.Loop || len(sc.Timeline) != 1 || sc.Width != DefaultScenario().Width {
		t.Fatalf("good file: sc=%+v err=%v", sc, err)
	}
	for name, body := range map[string]string{
		"bad.json":     `{not json`,
		"empty.json":   `{"timeline":[]}`,
		"nowidth.json": `{"width":0,"timeline":[{"phase":"empty","ms":1}]}`,
	} {
		if _, err := LoadScenario(write(name, body)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := LoadScenario(filepath.Join(dir, "missing.json")); err == nil {
		t.Errorf("missing file: expected an error")
	}
}
