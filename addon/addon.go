// Package addon carries the Bobber Marker addon inside the app, so the app can
// install it into the game and keep it current.
package addon

import (
	"bytes"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed BobberMarker
var files embed.FS

// Name is the addon's folder name under Interface\AddOns.
const Name = "BobberMarker"

// Dir returns where the addon lives for the game installed in gameDir, the
// folder that holds the game's exe (World of Warcraft\_forever_).
func Dir(gameDir string) string { return filepath.Join(gameDir, "Interface", "AddOns", Name) }

// Version returns the version in the carried addon's .toc file.
func Version() string {
	toc, _ := files.ReadFile(Name + "/" + Name + ".toc")
	for line := range strings.Lines(string(toc)) {
		if v, ok := strings.CutPrefix(line, "## Version:"); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// State reports whether the addon is installed in gameDir, and whether every
// file matches the carried one.
func State(gameDir string) (installed, current bool) {
	dir := Dir(gameDir)
	if _, err := os.Stat(filepath.Join(dir, Name+".toc")); err != nil {
		return false, false
	}
	current = true
	_ = fs.WalkDir(files, Name, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		want, _ := files.ReadFile(path)
		got, err := os.ReadFile(filepath.Join(dir, strings.TrimPrefix(path, Name+"/")))
		if err != nil || !bytes.Equal(got, want) {
			current = false
			return fs.SkipAll
		}
		return nil
	})
	return true, current
}

// Install writes the carried addon into gameDir, replacing its files. The
// player's addon settings live in the game's WTF folder and stay.
func Install(gameDir string) error {
	dir := Dir(gameDir)
	return fs.WalkDir(files, Name, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dir, strings.TrimPrefix(strings.TrimPrefix(path, Name), "/"))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := files.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
