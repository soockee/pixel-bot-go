package desktop

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

func TestAskForSupport(t *testing.T) {
	now := time.Date(2026, 11, 9, 20, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	never := time.Time{}
	tests := []struct {
		name              string
		run, total        int
		asked, opened     time.Time
		dryRun, wantAsked bool
	}{
		{"a good run, first time", 12, 40, never, never, false, true},
		{"a short run", 9, 40, never, never, false, false},
		{"too few catches so far", 12, 19, never, never, false, false},
		{"asked a week ago", 12, 40, now.Add(-7 * day), never, false, false},
		{"asked two weeks ago", 12, 40, now.Add(-14 * day), never, false, true},
		{"opened the page a month ago", 12, 40, now.Add(-30 * day), now.Add(-30 * day), false, false},
		{"opened the page three months ago", 12, 40, now.Add(-90 * day), now.Add(-90 * day), false, true},
		{"a dry run", 12, 40, never, never, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AskForSupport(tt.run, tt.total, tt.asked, tt.opened, now, tt.dryRun); got != tt.wantAsked {
				t.Errorf("AskForSupport = %v, want %v", got, tt.wantAsked)
			}
		})
	}
}

func TestOpenSupportOpensThePageHidesTheCardAndRemembers(t *testing.T) {
	cfg := config.DefaultConfig()
	view := NewView(NewFrames(), func(State) {})
	view.SetSupport(Support{Ask: true, RunCatches: 12, Catches: 40})
	loop := NewLoop(func() {})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx, time.Hour)
	var opened []string
	path := filepath.Join(t.TempDir(), "config.json")
	s := NewService(loop, view, Commands{OpenURL: func(u string) { opened = append(opened, u) }}, nil, cfg, path, slog.New(slog.DiscardHandler))

	s.OpenSupport()
	if len(opened) != 1 || opened[0] != SupportURL {
		t.Fatalf("opened %v, want [%s]", opened, SupportURL)
	}
	if s.State().Support.Ask {
		t.Error("the card still shows")
	}
	saved, err := config.Load(path)
	if err != nil || saved.SupportOpenedAt == 0 {
		t.Errorf("saved SupportOpenedAt = %v, err %v", saved, err)
	}
}
