package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewer(t *testing.T) {
	for _, tc := range []struct {
		tag, current string
		want         bool
	}{
		{"v1.0.8", "v1.0.7", true},
		{"v1.1.0", "v1.0.9", true},
		{"v2.0.0", "v1.9.9", true},
		{"v1.0.10", "v1.0.9", true},
		{"v1.0.7", "v1.0.7", false},
		{"v1.0.6", "v1.0.7", false},
		{"v1.0.8", "dev", false},
		{"v1.0.8", "", false},
		{"1.0.8", "v1.0.7", false},
		{"v1.0", "v1.0.7", false},
		{"v1.0.8-rc1", "v1.0.7", false},
	} {
		if got := Newer(tc.tag, tc.current); got != tc.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tc.tag, tc.current, got, tc.want)
		}
	}
}

// fakeGitHub serves a latest release whose build is exe and whose
// CHECKSUMS.txt lists sum for it.
func fakeGitHub(t *testing.T, tag string, exe []byte, sum string, assets func(base string) string) *Client {
	t.Helper()
	srv := httptest.NewTLSServer(nil)
	t.Cleanup(srv.Close)
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/latest", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"tag_name":%q,"assets":[%s]}`, tag, assets(srv.URL+"/dl/"+tag+"/"))
	})
	mux.HandleFunc("/dl/"+tag+"/"+AssetName, func(w http.ResponseWriter, _ *http.Request) { w.Write(exe) })
	mux.HandleFunc("/dl/"+tag+"/"+sumsName, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "%s  other.exe\n%s  %s\n", strings.Repeat("0", 64), sum, AssetName)
	})
	srv.Config.Handler = mux
	return &Client{API: srv.URL + "/repos/latest", Download: srv.URL + "/dl/", HTTP: srv.Client()}
}

func bothAssets(base string) string {
	return fmt.Sprintf(`{"name":"CHECKSUMS.txt","browser_download_url":"%s%s"},{"name":%q,"browser_download_url":"%s%s"}`,
		base, sumsName, AssetName, base, AssetName)
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestFetchVerifiesChecksum(t *testing.T) {
	exe := []byte("new build")
	for _, tc := range []struct {
		name, sum string
		ok        bool
	}{
		{"match", sha(exe), true},
		{"mismatch", sha([]byte("tampered")), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fakeGitHub(t, "v1.2.3", exe, tc.sum, bothAssets)
			r, err := c.Latest(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if r.Tag != "v1.2.3" {
				t.Fatalf("tag = %q", r.Tag)
			}
			path := filepath.Join(t.TempDir(), "next.exe")
			var done int64
			err = c.Fetch(context.Background(), r, path, func(d, _ int64) { done = d })
			got, readErr := os.ReadFile(path)
			if tc.ok {
				if err != nil || readErr != nil || string(got) != string(exe) || done != int64(len(exe)) {
					t.Fatalf("Fetch = %v, file %q (%v), progress %d", err, got, readErr, done)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
				t.Fatalf("Fetch = %v, want checksum mismatch", err)
			}
			if readErr == nil {
				t.Fatal("a build failing its checksum was kept")
			}
		})
	}
}

func TestLatestRefusesUnexpectedAssets(t *testing.T) {
	for name, assets := range map[string]func(string) string{
		"no build": func(base string) string {
			return fmt.Sprintf(`{"name":"CHECKSUMS.txt","browser_download_url":"%s%s"}`, base, sumsName)
		},
		"other host": func(base string) string {
			return strings.ReplaceAll(bothAssets(base), base+AssetName, "https://evil.example/"+AssetName)
		},
		"other tag": func(base string) string {
			return strings.ReplaceAll(bothAssets(base), "/v1.2.3/", "/v0.0.1/")
		},
		"plain http": func(base string) string {
			return strings.ReplaceAll(bothAssets(base), "https://", "http://")
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := fakeGitHub(t, "v1.2.3", nil, "", assets)
			if _, err := c.Latest(context.Background()); err == nil {
				t.Fatal("Latest accepted the release")
			}
		})
	}
}

func TestInstallAndCleanup(t *testing.T) {
	dir := t.TempDir()
	exe, next := filepath.Join(dir, "app.exe"), filepath.Join(dir, "app.exe.new")
	os.WriteFile(exe, []byte("old"), 0o755)
	os.WriteFile(next, []byte("new"), 0o755)
	if err := Install(exe, next); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "new" {
		t.Fatalf("exe holds %q", got)
	}
	if got, _ := os.ReadFile(exe + ".old"); string(got) != "old" {
		t.Fatalf("old build holds %q", got)
	}
	if err := Cleanup(exe); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
		t.Fatal("old build kept")
	}
	if err := Cleanup(exe); err != nil {
		t.Fatalf("Cleanup without an old build: %v", err)
	}
}
