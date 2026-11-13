package action

import "testing"

func TestParseVK(t *testing.T) {
	tests := []struct {
		key    string
		want   byte
		wantOK bool
	}{
		{"F1", 0x70, true},
		{"F3", 0x72, true},
		{"f12", 0x7B, true},
		{" R ", 'R', true},
		{"Mouse4", 0x05, true},
		{"mouse5", 0x06, true},
		{"Mouse3", 0, false},
		{"F", 'F', true},
		{"", 0, false},
		{"F0", 0, false},
		{"F13", 0, false},
		{"F+1", 0, false},
		{"Escape", 0, false},
		{"7", '7', true},
		{"0", '0', true},
		{"Shift-1", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, ok := ParseVK(tt.key)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("got (%#x, %v), want (%#x, %v)", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
