package cloud

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTokenRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "account.bin")
	if tok, err := LoadToken(path); tok != "" || err != nil {
		t.Fatalf("missing file: %q, %v", tok, err)
	}
	if err := SaveToken(path, "1//refresh"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) == "1//refresh" {
		t.Fatal("token stored in plain text")
	}
	if tok, err := LoadToken(path); tok != "1//refresh" || err != nil {
		t.Fatalf("LoadToken = %q, %v", tok, err)
	}
	if err := ForgetToken(path); err != nil {
		t.Fatal(err)
	}
	if tok, _ := LoadToken(path); tok != "" {
		t.Errorf("token %q after ForgetToken", tok)
	}
}
