package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/soocke/pixel-bot-go/config"
)

// sampleLog is one start: a cast that ran out, one with a splash, a bite and
// a catch, a planned miss, an ESP32 timeout, and a stop by the person.
const sampleLog = `{"time":"2026-11-21T12:00:00+01:00","level":"INFO","msg":"ESP32 keyboard ready","port":"COM3","buttons":true}
{"time":"2026-11-21T12:00:01+01:00","level":"INFO","msg":"cast action executed","key":"F9"}
{"time":"2026-11-21T12:00:01.5+01:00","level":"INFO","msg":"found blobber","x":1,"y":2}
{"time":"2026-11-21T12:00:12+01:00","level":"DEBUG","msg":"spray under the bar","share":0.05,"bar":0.09,"after_lock_ms":10500}
{"time":"2026-11-21T12:00:12.1+01:00","level":"DEBUG","msg":"spray under the bar","share":0.07,"bar":0.09,"after_lock_ms":10600}
{"time":"2026-11-21T12:00:12.3+01:00","level":"DEBUG","msg":"sound without a splash","after_lock_ms":10800}
{"time":"2026-11-21T12:00:31.5+01:00","level":"INFO","msg":"fishing state transition","from":"monitoring","to":"casting"}
{"time":"2026-11-21T12:00:32+01:00","level":"INFO","msg":"cast action executed","key":"F9"}
{"time":"2026-11-21T12:00:32.5+01:00","level":"INFO","msg":"found blobber","x":1,"y":2}
{"time":"2026-11-21T12:00:40+01:00","level":"DEBUG","msg":"splash seen","share":0.21,"centre":180,"all":190}
{"time":"2026-11-21T12:00:40.3+01:00","level":"INFO","msg":"bite detected","detector":"splash_sound","sound_after_splash_ms":310}
{"time":"2026-11-21T12:00:40.3+01:00","level":"INFO","msg":"fishing state transition","from":"monitoring","to":"reeling"}
{"time":"2026-11-21T12:00:41+01:00","level":"INFO","msg":"fishing outcome","outcome":"caught"}
{"time":"2026-11-21T12:00:42+01:00","level":"INFO","msg":"cast action executed","key":"F9"}
{"time":"2026-11-21T12:00:42.5+01:00","level":"INFO","msg":"found blobber","x":1,"y":2}
{"time":"2026-11-21T12:00:50+01:00","level":"INFO","msg":"bite detected","detector":"splash_sound","sound_after_splash_ms":-20}
{"time":"2026-11-21T12:00:50+01:00","level":"INFO","msg":"reel missed","reason":"forgot to reel"}
{"time":"2026-11-21T12:01:20+01:00","level":"WARN","msg":"halting: the ESP32 keyboard failed: ESP32 tap acknowledgment: i/o timeout"}
{"time":"2026-11-21T12:01:20+01:00","level":"INFO","msg":"fishing state transition","from":"casting","to":"halt"}
{"time":"2026-11-21T12:02:00+01:00","level":"INFO","msg":"fishing state transition","from":"focus","to":"halt"}
`

func TestRunSummaryTellsHowTheRunsWent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.json")
	// The current start has no casts yet: the one before is summarised.
	os.WriteFile(path, []byte(`{"time":"2026-11-21T13:00:00+01:00","level":"INFO","msg":"Go Fish started"}`+"\n"), 0o644)
	os.WriteFile(config.KeptLogPath(path, 1), []byte(sampleLog), 0o644)

	got := RunSummary(path)
	for _, want := range []string{
		"Log 12:00:00 to 12:02:00",
		"Board: buttons=true port=COM3",
		"Casts 3: bites 2, ran out without a bite 1. Outcomes: 1× caught. Planned misses: 1× forgot to reel.",
		"Stopped by itself: 1× the ESP32 keyboard failed: ESP32 tap acknowledgment: i/o timeout.",
		"Stopped by Stop, or by the game losing focus: 1.",
		"12:00:01 30.0s, splashes 0 (under the bar 0.07), sounds 1, ran out",
		"12:00:32 7.8s, splashes 1 (0.21), sounds 0, bite (sound +310 ms after the splash), caught",
		"bite (sound -20 ms after the splash), planned miss: forgot to reel",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("summary lacks %q:\n%s", want, got)
		}
	}
	if RunSummary(filepath.Join(dir, "none.json")) != "" {
		t.Error("a summary without any log")
	}
}
