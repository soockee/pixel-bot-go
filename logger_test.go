package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/soocke/pixel-bot-go/config"
)

func TestLogsOfTheLastStartsAreKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.json")
	read := func(n int) string {
		b, _ := os.ReadFile(config.KeptLogPath(path, n))
		return string(b)
	}
	for _, start := range []string{"first", "second", "third", "fourth"} {
		l, err := openLog(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := l.Write([]byte(start)); err != nil {
			t.Fatal(err)
		}
		l.f.Close()
	}
	if read(0) != "fourth" || read(1) != "third" || read(2) != "second" {
		t.Fatalf("kept %q, %q, %q; want the last three starts", read(0), read(1), read(2))
	}
	if _, err := os.Stat(config.KeptLogPath(path, 3)); !os.IsNotExist(err) {
		t.Error("kept a fourth log")
	}

	l, err := openLog(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { l.f.Close() }() // the file after the last rotation
	l.limit = 10
	l.Write([]byte("12345678"))
	l.Write([]byte("next")) // over the limit: a fresh file
	if read(0) != "next" || read(1) != "12345678" {
		t.Errorf("after the limit: %q, then %q", read(0), read(1))
	}
}
