package config

import "testing"

func TestKeyCode(t *testing.T) {
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
			got, ok := KeyCode(tt.key)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("got (%#x, %v), want (%#x, %v)", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestCheckKeys(t *testing.T) {
	tests := []struct {
		name string
		set  func(*Config)
		want string // "" for no error
	}{
		{"defaults", func(*Config) {}, ""},
		{"unknown Cast", func(c *Config) { c.CastKey = "Ctrl" }, `"Ctrl" is not a Cast key Go Fish can press`},
		{"Interact reel without a key", func(c *Config) { c.ReelMethod, c.InteractKey = ReelInteractKey, "" }, "set the Interact key before enabling this reel method"},
		{"Interact equals Cast", func(c *Config) { c.ReelMethod, c.InteractKey = ReelInteractKey, c.CastKey }, "the Interact key must differ from the Cast key"},
		{"unused Interact may equal Cast", func(c *Config) { c.InteractKey = c.CastKey }, ""},
		{"unused Interact must parse", func(c *Config) { c.InteractKey = "Ctrl" }, `"Ctrl" is not an Interact key Go Fish can press`},
		{"Logout equals Cast", func(c *Config) { c.LogoutKey = c.CastKey }, "the Logout key must differ from the Cast key"},
		{"Logout equals the Interact reel", func(c *Config) { c.ReelMethod, c.InteractKey, c.LogoutKey = ReelInteractKey, "L", "l" }, "the Logout key must differ from the Interact key"},
		{"Logout may equal an unused Interact", func(c *Config) { c.InteractKey, c.LogoutKey = "L", "l" }, ""},
		{"empty Logout is fine until a run", func(c *Config) { c.Schedule, c.LogoutKey = true, "" }, ""},
		{"reposition walks over Cast", func(c *Config) { c.Reposition, c.CastKey = true, "w" }, "repositioning walks with W, which is assigned to Cast"},
		{"WASD is free without reposition", func(c *Config) { c.CastKey = "w" }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := DefaultConfig()
			tt.set(c)
			err := c.CheckKeys()
			if got := errText(err); got != tt.want {
				t.Errorf("CheckKeys() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckRunKeysAsksForLogout(t *testing.T) {
	for _, on := range []func(*Config){
		func(c *Config) { c.Schedule = true },
		func(c *Config) { c.Reposition = true },
	} {
		c := DefaultConfig()
		on(c)
		c.LogoutKey = ""
		if got := errText(c.CheckRunKeys()); got != "set the Logout key: breaks and reposition log out" {
			t.Errorf("CheckRunKeys() = %q", got)
		}
	}
	c := DefaultConfig()
	c.LogoutKey = ""
	if err := c.CheckRunKeys(); err != nil {
		t.Errorf("a run without breaks or reposition needs no Logout key: %v", err)
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
