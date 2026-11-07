package view

import "testing"

func TestPickWindow(t *testing.T) {
	const wow = "World of Warcraft"
	tests := []struct {
		name    string
		titles  []string
		current string
		chosen  bool
		want    string
	}{
		{"defaults to the game", []string{"Claude", wow}, "", false, wow},
		{"falls back to the first window", []string{"Claude", "Notes"}, "", false, "Claude"},
		{"the game appears after a refresh", []string{"Claude", wow}, "Claude", false, wow},
		{"keeps a window the user chose", []string{"Claude", wow}, "Claude", true, "Claude"},
		{"keeps a chosen window that is not open", []string{wow}, "Fake Game", true, "Fake Game"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pickWindow(tt.titles, tt.current, tt.chosen); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
