package desktop

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/soocke/pixel-bot-go/internal/dataset"
	"github.com/soocke/pixel-bot-go/internal/recorder"
)

// RecordingsPath is where Recordings serves the newest frame of a recording,
// as RecordingsPath+name.
const RecordingsPath = "/recordings/"

// Recording is one recording directory.
type Recording struct {
	Name     string `json:"name"` // directory name, also its start time
	HasInput bool   `json:"hasInput"`
	HasAudio bool   `json:"hasAudio"`
}

// Recordings reads the recordings in one folder. Names from the frontend are
// directory names only, so nothing outside the folder can be read.
type Recordings struct {
	dir func() string // the folder, read on every call since settings can change it
}

// NewRecordings reads the recordings in the folder dir returns.
func NewRecordings(dir func() string) *Recordings { return &Recordings{dir: dir} }

// List returns the recordings, newest first.
func (r *Recordings) List() ([]Recording, error) {
	entries, err := os.ReadDir(r.dir())
	if errors.Is(err, os.ErrNotExist) {
		return []Recording{}, nil
	}
	if err != nil {
		return nil, err
	}
	list := []Recording{}
	for _, e := range slices.Backward(entries) {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(r.dir(), e.Name())
		list = append(list, Recording{Name: e.Name(), HasInput: exists(dataset.InputPath(dir)), HasAudio: exists(dataset.AudioPath(dir))})
	}
	return list, nil
}

// Events reads the input trace of recording name from offset on.
func (r *Recordings) Events(name string, offset int64) (recorder.EventPage, error) {
	dir, err := r.path(name)
	if err != nil {
		return recorder.EventPage{}, err
	}
	return recorder.ReadEvents(dataset.InputPath(dir), offset)
}

// ServeHTTP serves the newest frame of the recording named in the path.
func (r *Recordings) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	dir, err := r.path(strings.TrimPrefix(req.URL.Path, RecordingsPath))
	frame := ""
	if err == nil {
		frame = dataset.LatestFrame(dataset.FramesPath(dir))
	}
	if frame == "" {
		http.NotFound(w, req)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, req, frame)
}

func (r *Recordings) path(name string) (string, error) {
	if name == "" || name != filepath.Base(name) || name == "." || name == ".." {
		return "", errors.New("invalid recording name")
	}
	return filepath.Join(r.dir(), name), nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
