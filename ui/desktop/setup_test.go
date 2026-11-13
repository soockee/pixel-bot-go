package desktop

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

func TestSetupSteps(t *testing.T) {
	ready := State{Window: defaultWindow, GameFound: true, ESP32Port: "AUTO", Addon: "current"}
	cases := []struct {
		name       string
		state      State
		done       bool
		step       int    // the step to look at
		wantAction string // its button
		wantDetail string // part of its text
	}{
		{"nothing is running", State{Window: defaultWindow}, false, 0, "", "Waiting for the WoW window"},
		{"a chosen window is closed", State{Window: "Fake Game"}, false, 0, "", `"Fake Game" is not open`},
		{"game folder unknown", State{Window: defaultWindow}, false, 3, "", "find the game folder"},
		{"already open", State{Window: defaultWindow, GameFound: true}, false, 0, "", "Found the World of Warcraft window"},
		{"no board chosen", State{GameFound: true, Addon: "current"}, false, 1, "Find the board", "USB cable"},
		{"board found", State{GameFound: true, ESP32Port: "AUTO", BoardFound: "COM5"}, false, 1, "", "on COM5"},
		{"board on auto", State{GameFound: true, ESP32Port: "AUTO"}, false, 1, "", "when fishing starts"},
		{"board on a chosen port", State{GameFound: true, ESP32Port: "COM7"}, false, 1, "", "on COM7"},
		{"keys not chosen", State{GameFound: true, ESP32Port: "AUTO", Addon: "missing"}, false, 2, "Use these keys", "F9 to F12"},
		{"keys chosen", State{GameFound: true, ESP32Port: "AUTO", Addon: "missing", KeysChosen: true}, false, 2, "", "binds these keys"},
		{"addon missing", State{GameFound: true, ESP32Port: "AUTO", Addon: "missing", KeysChosen: true}, false, 3, "Install addon", "binds your keys"},
		{"addon outdated", State{GameFound: true, ESP32Port: "AUTO", Addon: "outdated"}, false, 3, "Update addon", "apply your keys"},
		{"a current addon needs no key choice", ready, true, 2, "", "binds these keys"},
		{"everything in place", ready, true, 3, "", "up to date"},
		{"a dry run needs none of it", State{DryRun: true}, true, 0, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			steps, done := setupSteps(tc.state)
			if done != tc.done {
				t.Errorf("done = %v, want %v: %+v", done, tc.done, steps)
			}
			got := steps[tc.step]
			if got.Action != tc.wantAction || !strings.Contains(got.Detail, tc.wantDetail) {
				t.Errorf("step %d = %+v, want action %q and detail containing %q", tc.step, got, tc.wantAction, tc.wantDetail)
			}
		})
	}
}

func TestSetupKeysAreTheKeysTheRunUses(t *testing.T) {
	cases := []struct {
		name string
		edit func(*config.Config)
		want [4]string
	}{
		{"right click", func(*config.Config) {}, [4]string{"castKey", "targetPlayerKey"}},
		{"interact reel", func(c *config.Config) { c.ReelMethod = config.ReelInteractKey }, [4]string{"castKey", "interactKey", "targetPlayerKey"}},
		{"breaks", func(c *config.Config) { c.Schedule = true }, [4]string{"castKey", "targetPlayerKey", "logoutKey"}},
		{"all of them", func(c *config.Config) { c.ReelMethod, c.Reposition = config.ReelInteractKey, true }, [4]string{"castKey", "interactKey", "targetPlayerKey", "logoutKey"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := config.DefaultConfig()
			tc.edit(c)
			if got := setupKeys(c); got != tc.want {
				t.Fatalf("setupKeys = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestViewFlushSendsTheSetupSteps(t *testing.T) {
	var sent []State
	v := NewView(NewFrames(), func(s State) { sent = append(sent, s) })
	v.SetWindow(defaultWindow)
	v.Flush()
	if len(sent) != 1 || sent[0].SetupDone || sent[0].Setup[0].ID != "game" {
		t.Fatalf("first state = %+v", sent)
	}
	v.SetGameFound(true)
	v.SetInput(&config.Config{ESP32Port: "AUTO"})
	v.SetAddon("dir", "current")
	v.Flush()
	if len(sent) != 2 || !sent[1].SetupDone {
		t.Fatalf("setup should be done once the game runs and the addon is current: %+v", sent[len(sent)-1])
	}
	v.Flush()
	if len(sent) != 2 {
		t.Fatal("an unchanged state was sent again")
	}
}

// gameService is a Service whose windows are the given titles; it records every window it selects.
func gameService(t *testing.T, titles []string, running bool) (*Service, *[]string) {
	t.Helper()
	view := NewView(NewFrames(), func(State) {})
	loop := NewLoop(func() {})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go loop.Run(ctx, time.Hour)
	selected := new([]string)
	cmds := Commands{
		ListWindows: func() []string { return titles },
		SetWindow:   func(title string) { view.SetWindow(title); *selected = append(*selected, title) },
		Running:     func() bool { return running },
	}
	s := NewService(loop, view, cmds, nil, config.DefaultConfig(), filepath.Join(t.TempDir(), "config.json"), slog.New(slog.DiscardHandler))
	view.SetWindow("Notepad")
	return s, selected
}

func TestCheckGameSelectsTheGameWhenItOpens(t *testing.T) {
	s, selected := gameService(t, []string{"Notepad", defaultWindow}, false)
	s.CheckGame()
	got := s.State()
	if got.Window != defaultWindow || !got.GameFound || len(*selected) != 1 {
		t.Fatalf("window %q, found %v, selected %v; want the game selected once", got.Window, got.GameFound, *selected)
	}
}

func TestCheckGameKeepsAWindowThePersonChose(t *testing.T) {
	s, selected := gameService(t, []string{"Notepad", defaultWindow}, false)
	s.SetWindow("Notepad")
	s.CheckGame()
	got := s.State()
	if got.Window != "Notepad" || !got.GameFound || len(*selected) != 1 {
		t.Fatalf("window %q, found %v, selected %v; want the chosen window kept", got.Window, got.GameFound, *selected)
	}
}

func TestCheckGameDoesNotSwitchDuringARun(t *testing.T) {
	s, selected := gameService(t, []string{"Notepad", defaultWindow}, true)
	s.CheckGame()
	if got := s.State().Window; got != "Notepad" || len(*selected) != 0 {
		t.Fatalf("window %q, selected %v; a run keeps its window", got, *selected)
	}
}

func TestCheckGameReportsAClosedWindow(t *testing.T) {
	s, _ := gameService(t, []string{"Explorer"}, false)
	s.CheckGame()
	if s.State().GameFound {
		t.Fatal("GameFound with the selected window closed")
	}
}

func TestFindBoardLooksOnTheSavedPortOrEverywhere(t *testing.T) {
	for _, tc := range []struct{ saved, want string }{{"", "AUTO"}, {"COM7", "COM7"}} {
		view := NewView(NewFrames(), func(State) {})
		loop := NewLoop(func() {})
		ctx, cancel := context.WithCancel(context.Background())
		go loop.Run(ctx, time.Hour)
		cfg := config.DefaultConfig()
		cfg.ESP32Port = tc.saved
		var asked string
		cmds := Commands{Running: func() bool { return false }, FindBoard: func(port string) (string, error) { asked = port; return "COM5", nil }}
		s := NewService(loop, view, cmds, nil, cfg, filepath.Join(t.TempDir(), "config.json"), slog.New(slog.DiscardHandler))
		found, err := s.FindBoard()
		if err != nil || found != "COM5" || asked != tc.want || s.State().BoardFound != "COM5" {
			t.Errorf("saved %q: found %q, %v, asked %q, state %q", tc.saved, found, err, asked, s.State().BoardFound)
		}
		cancel()
	}
}

func TestFindBoardWaitsForTheRunToStop(t *testing.T) {
	s, _ := gameService(t, nil, true)
	s.cmds.FindBoard = func(string) (string, error) { t.Fatal("opened the port during a run"); return "", nil }
	if _, err := s.FindBoard(); err == nil {
		t.Fatal("FindBoard ran during a run")
	}
}
