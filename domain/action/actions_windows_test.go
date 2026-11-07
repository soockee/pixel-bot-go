package action

import (
	"testing"
	"unsafe"
)

func TestInputStructsMatchWin32Size(t *testing.T) {
	const want = 40 // sizeof(INPUT) on 64-bit Windows
	if got := unsafe.Sizeof(keyboardInput{}); got != want {
		t.Errorf("keyboardInput size = %d, want %d", got, want)
	}
	if got := unsafe.Sizeof(mouseInput{}); got != want {
		t.Errorf("mouseInput size = %d, want %d", got, want)
	}
}

func TestInputUnionOffsets(t *testing.T) {
	if got := unsafe.Offsetof(keyboardInput{}.vk); got != 8 {
		t.Errorf("keyboard union starts at %d, want 8", got)
	}
	if got := unsafe.Offsetof(mouseInput{}.flags); got != 20 {
		t.Errorf("mouse flags at %d, want 20", got)
	}
}

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
		{"F", 'F', true},
		{"", 0, false},
		{"F0", 0, false},
		{"F13", 0, false},
		{"F+1", 0, false},
		{"Escape", 0, false},
		{"7", 0, false},
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
