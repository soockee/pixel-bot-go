package addon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var keys = Keys{Cast: "F3", Interact: "F"}

func TestInstallAndState(t *testing.T) {
	game := t.TempDir()
	if installed, _ := State(game, keys); installed {
		t.Fatal("installed before Install")
	}
	if err := Install(game, keys); err != nil {
		t.Fatal(err)
	}
	if installed, current := State(game, keys); !installed || !current {
		t.Fatalf("after Install: installed %v, current %v", installed, current)
	}

	lua := filepath.Join(Dir(game), Name+".lua")
	if err := os.WriteFile(lua, []byte("-- old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if installed, current := State(game, keys); !installed || current {
		t.Fatalf("changed file: installed %v, current %v", installed, current)
	}
	if err := Install(game, keys); err != nil {
		t.Fatal(err)
	}
	if _, current := State(game, keys); !current {
		t.Fatal("Install did not replace the changed file")
	}
}

func TestKeysFollowTheSettings(t *testing.T) {
	game := t.TempDir()
	if err := Install(game, keys); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(Dir(game), keysFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `cast = "F3", interact = "F"`) {
		t.Fatalf("Keys.lua = %q", got)
	}
	// A key changed in Settings makes the installed addon outdated until it is installed again.
	if _, current := State(game, Keys{Cast: "F4", Interact: "F"}); current {
		t.Fatal("a changed Cast key left the addon current")
	}
	if _, current := State(game, Keys{Cast: "F3"}); current {
		t.Fatal("a removed Interact key left the addon current")
	}
	if err := Install(game, Keys{Cast: "F4"}); err != nil {
		t.Fatal(err)
	}
	if _, current := State(game, Keys{Cast: "F4"}); !current {
		t.Fatal("Install did not write the new keys")
	}
}

func TestTocLoadsTheKeysBeforeTheAddon(t *testing.T) {
	toc, err := files.ReadFile(Name + "/" + Name + ".toc")
	if err != nil {
		t.Fatal(err)
	}
	keysAt, addonAt := strings.Index(string(toc), keysFile), strings.Index(string(toc), Name+".lua")
	if keysAt < 0 || addonAt < 0 || keysAt > addonAt {
		t.Fatalf("the .toc must list %s before %s.lua:\n%s", keysFile, Name, toc)
	}
}

func TestBindingKey(t *testing.T) {
	for in, want := range map[string]string{
		"F3": "F3", "f12": "F12", " f1 ": "F1", "a": "A", "7": "7",
		"Mouse4": "BUTTON4", "MOUSE5": "BUTTON5",
		"": "", "F0": "", "F13": "", "F03": "", "Ctrl": "", "Space": "", "ab": "", "Mouse3": "",
	} {
		if got := BindingKey(in); got != want {
			t.Errorf("BindingKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVersion(t *testing.T) {
	if Version() == "" {
		t.Fatal("no version in the carried .toc")
	}
}
