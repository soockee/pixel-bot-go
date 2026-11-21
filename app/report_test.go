package app

import (
	"archive/zip"
	"io"
	"maps"
	"os"
	"slices"
	"testing"

	"github.com/soocke/pixel-bot-go/config"
)

func TestReportHoldsTheSummaryLogsAndSettings(t *testing.T) {
	t.Chdir(t.TempDir())
	os.WriteFile(config.FilePath, []byte(`{"cast_key":"F9"}`), 0o644)
	os.WriteFile(config.LogPath, []byte("now\n"), 0o644)
	os.WriteFile(config.KeptLogPath(config.LogPath, 1), []byte("before\n"), 0o644)

	if err := writeReport("report.zip", "what the issue says"); err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader("report.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	got := map[string]string{}
	for _, f := range z.File {
		r, _ := f.Open()
		b, _ := io.ReadAll(r)
		r.Close()
		got[f.Name] = string(b)
	}
	want := map[string]string{
		"summary.txt":                         "what the issue says",
		config.FilePath:                       `{"cast_key":"F9"}`,
		config.LogPath:                        "now\n",
		config.KeptLogPath(config.LogPath, 1): "before\n",
	}
	for name, body := range want {
		if got[name] != body {
			t.Errorf("%s = %q, want %q", name, got[name], body)
		}
	}
	if len(got) != len(want) {
		t.Errorf("report holds %v, want only the files that exist", slices.Sorted(maps.Keys(got)))
	}
}
