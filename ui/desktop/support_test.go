package desktop

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

func TestSupportMilestone(t *testing.T) {
	good := SupportRun{Catches: 12}
	tests := []struct {
		name          string
		total, asked  int
		run           SupportRun
		muted, dryRun bool
		want          int
	}{
		{"before the first milestone", 99, 0, good, false, false, 0},
		{"the first milestone", 104, 0, good, false, false, 100},
		{"already asked at it", 180, 100, good, false, false, 0},
		{"the next milestone", 503, 100, good, false, false, 500},
		{"two milestones in one run ask once, at the higher", 1010, 100, good, false, false, 1000},
		{"after the list, every 5000", 10020, 5000, good, false, false, 10000},
		{"a run without a catch", 120, 0, SupportRun{}, false, false, 0},
		{"a run with many false bites", 120, 0, SupportRun{Catches: 8, NotHooked: 3}, false, false, 0},
		{"one false bite per four catches is fine", 120, 0, SupportRun{Catches: 8, NotHooked: 2}, false, false, 100},
		{"muted", 1000, 0, good, true, false, 0},
		{"a dry run", 1000, 0, good, false, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := SupportMilestone(tt.total, tt.asked, tt.run, tt.muted, tt.dryRun)
			if got != tt.want || ok != (tt.want != 0) {
				t.Errorf("SupportMilestone = %d, %v; want %d", got, ok, tt.want)
			}
		})
	}
}

func newSupportService(t *testing.T) (*Service, *View, *config.Config, string, *[]string) {
	t.Helper()
	cfg := config.DefaultConfig()
	view := NewView(NewFrames(), func(State) {})
	view.SetSupport(Support{Ask: true, Milestone: 100, Catches: 104})
	loop := NewLoop(func() {})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go loop.Run(ctx, time.Hour)
	var opened []string
	path := filepath.Join(t.TempDir(), "config.json")
	s := NewService(loop, view, Commands{OpenURL: func(u string) { opened = append(opened, u) }}, nil, cfg, path, slog.New(slog.DiscardHandler))
	return s, view, cfg, path, &opened
}

func TestOpenSupportOpensThePageAndHidesTheCard(t *testing.T) {
	s, _, _, _, opened := newSupportService(t)
	s.OpenSupport()
	if len(*opened) != 1 || (*opened)[0] != SupportURL {
		t.Fatalf("opened %v, want [%s]", *opened, SupportURL)
	}
	if s.State().Support.Ask {
		t.Error("the card still shows")
	}
}

func TestMuteSupportStopsTheCardForGood(t *testing.T) {
	s, _, cfg, path, opened := newSupportService(t)
	s.MuteSupport()
	if s.State().Support.Ask || len(*opened) != 0 {
		t.Fatalf("card shows %v, opened %v", s.State().Support.Ask, *opened)
	}
	saved, err := config.Load(path)
	if err != nil || !saved.SupportMuted || !cfg.SupportMuted {
		t.Errorf("saved muted = %v, err %v", saved != nil && saved.SupportMuted, err)
	}
}
