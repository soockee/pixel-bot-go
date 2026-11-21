package app

import (
	"archive/zip"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

// writeReport saves a report file at path for a bug report: summary.txt with
// details (what Report a problem fills in), the kept logs and the config
// file, where they exist. Nothing leaves the PC: the person drags the file into
// the issue when asked to.
func writeReport(path, details string) (err error) {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(path)
		}
	}()
	z := zip.NewWriter(out)
	w, err := z.CreateHeader(&zip.FileHeader{Name: "summary.txt", Method: zip.Deflate, Modified: time.Now()})
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, details); err != nil {
		return err
	}
	files := []string{config.FilePath}
	for n := range config.LogsKept {
		files = append(files, config.KeptLogPath(config.LogPath, n))
	}
	for _, name := range files {
		if err := addFile(z, name); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return z.Close()
}

func addFile(z *zip.Writer, name string) error {
	in, err := os.Open(name)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	// The entry keeps the file's time, so a log tells when it was written.
	w, err := z.CreateHeader(&zip.FileHeader{Name: filepath.Base(name), Method: zip.Deflate, Modified: info.ModTime()})
	if err != nil {
		return err
	}
	_, err = io.Copy(w, in)
	return err
}
