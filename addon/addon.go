// Package addon carries the Bobber Marker addon inside the app, so the app can
// install it into the game and keep it current.
package addon

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The files are named, not the folder: a game folder linked to the source
// folder gets keysFile written into it by Install, which must not be carried.
//
//go:embed BobberMarker/BobberMarker.lua BobberMarker/BobberMarker.toc
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

// keysFile is written at install time, not carried: it holds the keys Go Fish
// presses, which the addon binds in the game.
const keysFile = "Keys.lua"

// Keys are the bindings Go Fish presses, in the game's names (see BindingKey).
// An empty key is not bound.
type Keys struct{ Cast, Interact, TargetPlayer string }

// BindingKey converts a Go Fish key name (a letter, a digit, F1 to F12, Mouse4
// or Mouse5) into the name the game uses for a binding. It returns "" for any
// other name, which the addon leaves unbound.
func BindingKey(name string) string {
	k := strings.ToUpper(strings.TrimSpace(name))
	switch {
	case k == "MOUSE4":
		return "BUTTON4"
	case k == "MOUSE5":
		return "BUTTON5"
	case len(k) == 1 && (k[0] >= 'A' && k[0] <= 'Z' || k[0] >= '0' && k[0] <= '9'):
		return k
	}
	if n, ok := strings.CutPrefix(k, "F"); ok {
		if v, err := strconv.Atoi(n); err == nil && v >= 1 && v <= 12 && n == strconv.Itoa(v) {
			return k
		}
	}
	return ""
}

func keysLua(k Keys) []byte {
	return fmt.Appendf(nil, "-- Written by Go Fish when it installs the addon: the keys it presses.\n"+
		"BobberMarkerKeys = { cast = %q, interact = %q, targetPlayer = %q };\n", k.Cast, k.Interact, k.TargetPlayer)
}

// State reports whether the addon is installed in gameDir, and whether every
// file, and the keys, match what Install would write.
func State(gameDir string, keys Keys) (installed, current bool) {
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
	if got, err := os.ReadFile(filepath.Join(dir, keysFile)); err != nil || !bytes.Equal(got, keysLua(keys)) {
		current = false
	}
	return true, current
}

// Install writes the carried addon and the keys into gameDir, replacing its
// files. The player's addon settings live in the game's WTF folder and stay.
func Install(gameDir string, keys Keys) error {
	dir := Dir(gameDir)
	err := fs.WalkDir(files, Name, func(path string, d fs.DirEntry, err error) error {
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
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, keysFile), keysLua(keys), 0o644)
}
