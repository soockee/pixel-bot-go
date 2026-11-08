package addon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallAndState(t *testing.T) {
	game := t.TempDir()
	if installed, _ := State(game); installed {
		t.Fatal("installed before Install")
	}
	if err := Install(game); err != nil {
		t.Fatal(err)
	}
	if installed, current := State(game); !installed || !current {
		t.Fatalf("after Install: installed %v, current %v", installed, current)
	}

	lua := filepath.Join(Dir(game), Name+".lua")
	if err := os.WriteFile(lua, []byte("-- old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if installed, current := State(game); !installed || current {
		t.Fatalf("changed file: installed %v, current %v", installed, current)
	}
	if err := Install(game); err != nil {
		t.Fatal(err)
	}
	if _, current := State(game); !current {
		t.Fatal("Install did not replace the changed file")
	}
}

func TestVersion(t *testing.T) {
	if Version() == "" {
		t.Fatal("no version in the carried .toc")
	}
}
