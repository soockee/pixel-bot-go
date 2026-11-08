package app

import (
	"testing"

	"github.com/soocke/pixel-bot-go/addon"
	"github.com/soocke/pixel-bot-go/config"
)

func TestAddonKeysFollowTheReelMethod(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.CastKey, cfg.InteractKey = "f3", "f"
	a := &app{container: &AppContainer{Config: cfg}}

	// Right-click reeling never presses the Interact key, so the addon leaves it alone.
	if got, want := a.addonKeys(), (addon.Keys{Cast: "F3"}); got != want {
		t.Errorf("right-click reeling: keys = %+v, want %+v", got, want)
	}
	cfg.ReelMethod = config.ReelInteractKey
	if got, want := a.addonKeys(), (addon.Keys{Cast: "F3", Interact: "F"}); got != want {
		t.Errorf("interact reeling: keys = %+v, want %+v", got, want)
	}
	cfg.InteractKey = "Ctrl" // not a key the addon can bind
	if got := a.addonKeys().Interact; got != "" {
		t.Errorf("unbindable Interact key = %q, want none", got)
	}
}
