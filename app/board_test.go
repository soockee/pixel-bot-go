package app

import (
	"errors"
	"testing"

	"github.com/soocke/pixel-bot-go/config"
)

func TestBoardOwnerRefusesASecondUser(t *testing.T) {
	var o boardOwner
	o.mu.Lock() // a run holds the Board
	if _, _, err := o.open("COM999"); !errors.Is(err, errBoardBusy) {
		t.Fatalf("open while held: err = %v, want errBoardBusy", err)
	}
	o.mu.Unlock()
}

func TestBoardOwnerLetsGoAfterAFailedOpen(t *testing.T) {
	var o boardOwner
	if _, _, err := o.open("COM999"); err == nil || errors.Is(err, errBoardBusy) {
		t.Fatalf("open of a missing port: err = %v, want a port error", err)
	}
	if !o.mu.TryLock() {
		t.Fatal("a failed open kept the Board taken")
	}
}

func TestNeedsFor(t *testing.T) {
	tests := []struct {
		name      string
		set       func(*config.Config)
		timedTaps bool
		want      boardNeeds
	}{
		{"right-click reel points", func(*config.Config) {}, false, boardNeeds{rightClick: true}},
		{"Interact reel needs no pointer", func(c *config.Config) { c.ReelMethod = config.ReelInteractKey }, false, boardNeeds{}},
		{"reposition drags the camera", func(c *config.Config) { c.ReelMethod, c.Reposition = config.ReelInteractKey, true }, false, boardNeeds{camera: true}},
		{"idle moves wander", func(c *config.Config) { c.ReelMethod, c.IdleMoves = config.ReelInteractKey, true }, false, boardNeeds{wander: true}},
		{"a timing profile times taps", func(*config.Config) {}, true, boardNeeds{rightClick: true, timedTaps: true}},
		{"Mouse 4 as Cast", func(c *config.Config) { c.CastKey = "Mouse4" }, false, boardNeeds{rightClick: true, sideButtons: true}},
		{"Mouse 5 as Interact only with that reel", func(c *config.Config) { c.InteractKey = "Mouse5" }, false, boardNeeds{rightClick: true}},
		{"Mouse 5 as the Interact reel", func(c *config.Config) { c.ReelMethod, c.InteractKey = config.ReelInteractKey, "Mouse5" }, false, boardNeeds{sideButtons: true}},
		{"Mouse 4 as Logout only with breaks", func(c *config.Config) { c.LogoutKey = "Mouse4" }, false, boardNeeds{rightClick: true}},
		{"Mouse 4 as Logout with breaks", func(c *config.Config) { c.LogoutKey, c.Schedule = "Mouse4", true }, false, boardNeeds{rightClick: true, sideButtons: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			tt.set(cfg)
			if got := needsFor(*cfg, tt.timedTaps); got != tt.want {
				t.Errorf("needsFor = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPortOf(t *testing.T) {
	for in, want := range map[string]string{"": "select or connect the ESP32 board in Settings", "com3": "", "COM0": "enter a COM port such as COM3, or auto to find the board"} {
		got, err := portOf(config.Config{ESP32Port: in})
		if want == "" && (err != nil || got != "COM3") || want != "" && (err == nil || err.Error() != want) {
			t.Errorf("portOf(%q) = %q, %v", in, got, err)
		}
	}
}
