package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/internal/update"
)

func TestFishSpeciesAreUnique(t *testing.T) {
	seen := make(map[string]bool, len(fishSpecies))
	for _, species := range fishSpecies {
		if species == "" || strings.ContainsAny(species, `\\/:`) || seen[species] {
			t.Fatalf("invalid or repeated fish species %q", species)
		}
		seen[species] = true
	}
	if len(seen) < 30 {
		t.Fatalf("only %d fish species available", len(seen))
	}
}

func TestFishWindowTitleMatchesProcess(t *testing.T) {
	t.Setenv(update.FishNameEnv, "loach")
	if got := fishWindowTitle(); got != "loach" {
		t.Fatalf("fishWindowTitle() = %q, want %q", got, "loach")
	}
}

func TestLauncherStartsFishNamedCopy(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	process, dir, err := startFishCopy(exe, []string{"-test.run=TestFishChildUsesStableExecutable"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	waiter, err := os.FindProcess(process.Pid)
	if err != nil {
		_ = process.Release()
		t.Fatal(err)
	}
	if err := process.Release(); err != nil {
		_ = waiter.Release()
		t.Fatal(err)
	}
	state, err := waiter.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if !state.Success() {
		t.Fatalf("fish-named test process exited with %s", state)
	}
}

func TestCleanupFishCopiesRemovesOnlyOldCopies(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "pixel-bot-go-fish-old")
	fresh := filepath.Join(root, "pixel-bot-go-fish-fresh")
	otherContents := filepath.Join(root, "pixel-bot-go-fish-other")
	unrelated := filepath.Join(root, "other-temp-folder")
	for _, dir := range []string{old, fresh, otherContents, unrelated} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{filepath.Join(old, "loach.exe"), filepath.Join(otherContents, "notes.txt")} {
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	stale := time.Now().Add(-25 * time.Hour)
	for _, dir := range []string{old, otherContents, unrelated} {
		if err := os.Chtimes(dir, stale, stale); err != nil {
			t.Fatal(err)
		}
	}
	cleanupFishCopies(root)
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old launch folder still exists: %v", err)
	}
	for _, path := range []string{fresh, filepath.Join(otherContents, "notes.txt"), unrelated} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("cleanup removed or could not find %q: %v", path, err)
		}
	}
}

func TestFishChildUsesStableExecutable(t *testing.T) {
	if os.Getenv(update.FishChildEnv) != "1" {
		return // The launcher subprocess runs this test in its fish-named copy.
	}
	stable, err := update.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	name := strings.TrimSuffix(filepath.Base(child), filepath.Ext(child))
	if !slices.Contains(fishSpecies[:], name) {
		t.Fatalf("child executable name %q is not a fish species", name)
	}
	if os.Getenv(update.FishNameEnv) != name {
		t.Fatalf("window name %q does not match process name %q", os.Getenv(update.FishNameEnv), name)
	}
	if strings.EqualFold(filepath.Clean(stable), filepath.Clean(child)) {
		t.Fatalf("stable executable %q is the fish-named child itself", stable)
	}
	if _, err := os.Stat(stable); err != nil {
		t.Fatalf("stable executable path is unavailable: %v", err)
	}
}
