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
	ready := State{Window: defaultWindow, GameFound: true, Addon: "current"}
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
		{"game folder unknown", State{Window: defaultWindow}, false, 2, "", "find the game folder"},
		{"already open", State{Window: defaultWindow, GameFound: true}, false, 0, "", "Found the World of Warcraft window"},
		{"cast key not chosen", State{GameFound: true, Addon: "missing"}, false, 1, "Use this key", "Keep F3"},
		{"cast key chosen", State{GameFound: true, Addon: "missing", CastKeyChosen: true}, false, 1, "", "puts Fishing on this key"},
		{"addon missing", State{GameFound: true, Addon: "missing", CastKeyChosen: true}, false, 2, "Install addon", "binds your Cast key"},
		{"addon outdated", State{GameFound: true, Addon: "outdated"}, false, 2, "Update addon", "apply your keys"},
		{"a current addon needs no key choice", ready, true, 1, "", "puts Fishing on this key"},
		{"everything in place", ready, true, 2, "", "up to date"},
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

func TestViewFlushSendsTheSetupSteps(t *testing.T) {
	var sent []State
	v := NewView(NewFrames(), func(s State) { sent = append(sent, s) })
	v.SetWindow(defaultWindow)
	v.Flush()
	if len(sent) != 1 || sent[0].SetupDone || sent[0].Setup[0].ID != "game" {
		t.Fatalf("first state = %+v", sent)
	}
	v.SetGameFound(true)
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
