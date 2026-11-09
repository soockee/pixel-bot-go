package app

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/soocke/pixel-bot-go/addon"
	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/capture"
)

// defaultGameDir is where the Battle.net launcher installs WoW: Forever.
const defaultGameDir = `C:\Program Files (x86)\World of Warcraft\_forever_`

var errNoGame = errors.New("start World of Warcraft: Forever, so Go Fish can find its folder")

// gameDir returns the folder of the running game's exe, else the default
// install folder when it exists.
func (a *app) gameDir() (string, error) {
	exe, err := capture.WindowExe(a.container.Capture.Window())
	// Only a WoW exe counts, not whatever other window is selected.
	if err == nil && strings.HasPrefix(strings.ToLower(filepath.Base(exe)), "wow") {
		return filepath.Dir(exe), nil
	}
	if _, err := os.Stat(defaultGameDir); err == nil {
		return defaultGameDir, nil
	}
	return "", errNoGame
}

// addonKeys are the bindings the addon sets up in the game: the Cast key
// always, the Interact key only when it is the reel method.
func (a *app) addonKeys() addon.Keys {
	cfg := a.container.Config
	keys := addon.Keys{Cast: addon.BindingKey(cfg.CastKey), TargetPlayer: addon.BindingKey(cfg.TargetPlayerKey)}
	if cfg.ReelMethod == config.ReelInteractKey {
		keys.Interact = addon.BindingKey(cfg.InteractKey)
	}
	return keys
}

// checkAddon shows whether the addon is installed and current. It runs on
// the loop.
func (a *app) checkAddon() {
	dir, err := a.gameDir()
	if err != nil {
		a.container.View.SetAddon("", "")
		return
	}
	status := "missing"
	if installed, current := addon.State(dir, a.addonKeys()); current {
		status = "current"
	} else if installed {
		status = "outdated"
	}
	a.container.View.SetAddon(addon.Dir(dir), status)
}

// installAddon writes the carried addon into the game. It runs on the loop.
func (a *app) installAddon() error {
	dir, err := a.gameDir()
	if err == nil {
		err = addon.Install(dir, a.addonKeys())
	}
	if errors.Is(err, fs.ErrPermission) {
		err = errors.New("Windows refused to write to the game folder; start Go Fish as administrator")
	}
	a.checkAddon()
	if err != nil {
		a.logger.Warn("addon install failed", "error", err)
		a.container.View.SetStatus("Addon not installed: " + err.Error())
		return err
	}
	a.logger.Info("addon installed", "dir", addon.Dir(dir), "version", addon.Version())
	a.container.View.SetStatus("Bobber Marker installed. Restart the game or type /reload in it, and it sets up your Cast key.")
	return nil
}
