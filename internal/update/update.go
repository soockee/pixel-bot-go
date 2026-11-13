// Package update finds a newer GitHub Release of the app, downloads its
// Windows build, checks it against the release's CHECKSUMS.txt and swaps it
// for the running executable.
package update

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Version is the running build's release tag. Release builds set it with
// -ldflags "-X github.com/soocke/pixel-bot-go/internal/update.Version=vX.Y.Z";
// any other value turns the update check off.
var Version = "dev"

const (
	// AssetName is the Windows build attached to every release.
	AssetName = "pixel-bot-go-windows-amd64.exe"
	sumsName  = "CHECKSUMS.txt"
	maxSize   = 256 << 20 // refuse anything larger than any plausible build

	// InstallPathEnv is set by the fish-named launcher so updates replace the
	// stable executable instead of the temporary copy that is currently running.
	InstallPathEnv = "PIXELBOT_INSTALL_EXE"
	// FishChildEnv marks the temporary fish-named process started by the launcher.
	FishChildEnv = "PIXELBOT_FISH_CHILD"
	// FishNameEnv carries the selected species into the app for its window title.
	FishNameEnv = "PIXELBOT_FISH_NAME"
)

// Client talks to the GitHub Releases API of one repository.
type Client struct {
	API      string // latest-release endpoint
	Download string // the only URL prefix assets may come from
	HTTP     *http.Client
}

// releases is the public repository's release page.
const releases = "https://github.com/soockee/pixel-bot-go/releases/"

// New returns a client for the public soockee/pixel-bot-go repository.
func New() *Client {
	return &Client{
		API:      "https://api.github.com/repos/soockee/pixel-bot-go/releases/latest",
		Download: releases + "download/",
		HTTP:     &http.Client{CheckRedirect: httpsOnly},
	}
}

// httpsOnly refuses redirects off HTTPS; GitHub sends asset downloads to its CDN.
func httpsOnly(req *http.Request, via []*http.Request) error {
	if req.URL.Scheme != "https" {
		return fmt.Errorf("refusing redirect to %s", req.URL.Redacted())
	}
	if len(via) >= 10 {
		return errors.New("too many redirects")
	}
	return nil
}

// Release is a published release with the two files an update needs.
type Release struct {
	Tag       string
	Published time.Time
	exeURL    string
	sumsURL   string
}

// Notes returns the release page, which lists what changed.
func (r Release) Notes() string { return releases + "tag/" + r.Tag }

// Latest returns the latest published release.
func (c *Client) Latest(ctx context.Context) (Release, error) {
	body, err := c.get(ctx, c.API, "application/vnd.github+json")
	if err != nil {
		return Release{}, err
	}
	defer body.Close()
	var r struct {
		Tag       string    `json:"tag_name"`
		Published time.Time `json:"published_at"`
		Assets    []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(body, 1<<20)).Decode(&r); err != nil {
		return Release{}, fmt.Errorf("read release: %w", err)
	}
	if _, ok := parse(r.Tag); !ok {
		return Release{}, fmt.Errorf("latest release tag %q is not vX.Y.Z", r.Tag)
	}
	rel := Release{Tag: r.Tag, Published: r.Published}
	for _, a := range r.Assets {
		switch a.Name {
		case AssetName:
			rel.exeURL = a.URL
		case sumsName:
			rel.sumsURL = a.URL
		}
	}
	prefix := c.Download + r.Tag + "/"
	if !strings.HasPrefix(rel.exeURL, prefix) || !strings.HasPrefix(rel.sumsURL, prefix) {
		return Release{}, fmt.Errorf("release %s has no %s and %s under %s", r.Tag, AssetName, sumsName, prefix)
	}
	return rel, nil
}

// Fetch downloads the release's build to path and verifies its SHA-256
// against CHECKSUMS.txt. On any failure path is removed.
func (c *Client) Fetch(ctx context.Context, r Release, path string, progress func(done, total int64)) (err error) {
	want, err := c.checksum(ctx, r.sumsURL)
	if err != nil {
		return err
	}
	body, err := c.get(ctx, r.exeURL, "application/octet-stream")
	if err != nil {
		return err
	}
	defer body.Close()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(path)
		}
	}()
	h := sha256.New()
	w := &counter{w: io.MultiWriter(f, h), total: body.size, progress: progress}
	n, err := io.Copy(w, io.LimitReader(body, maxSize+1))
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	if n > maxSize {
		return errors.New("download is larger than any build")
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("checksum mismatch: got %s, CHECKSUMS.txt says %s", got, want)
	}
	return nil
}

// checksum returns the SHA-256 that CHECKSUMS.txt (sha256sum output) lists for AssetName.
func (c *Client) checksum(ctx context.Context, url string) (string, error) {
	body, err := c.get(ctx, url, "text/plain")
	if err != nil {
		return "", err
	}
	defer body.Close()
	s := bufio.NewScanner(io.LimitReader(body, 1<<16))
	for s.Scan() {
		sum, name, ok := strings.Cut(s.Text(), " ")
		name = strings.TrimLeft(name, " *")
		if ok && name == AssetName && len(sum) == 64 {
			if _, err := hex.DecodeString(sum); err == nil {
				return strings.ToLower(sum), nil
			}
		}
	}
	return "", fmt.Errorf("%s lists no checksum for %s", sumsName, AssetName)
}

type response struct {
	io.ReadCloser
	size int64 // -1 when unknown
}

func (c *Client) get(ctx context.Context, url, accept string) (*response, error) {
	if !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("refusing non-HTTPS URL %s", url)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return &response{resp.Body, resp.ContentLength}, nil
}

// counter reports how much was written.
type counter struct {
	w           io.Writer
	done, total int64
	progress    func(done, total int64)
}

func (c *counter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.done += int64(n)
	if c.progress != nil {
		c.progress(c.done, c.total)
	}
	return n, err
}

// Newer reports whether tag is a later vX.Y.Z than current. A current that
// is not a release tag (a local build) is never updated.
func Newer(tag, current string) bool {
	t, ok1 := parse(tag)
	c, ok2 := parse(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := range t {
		if t[i] != c[i] {
			return t[i] > c[i]
		}
	}
	return false
}

// IsRelease reports whether version is a release tag vX.Y.Z.
func IsRelease(version string) bool {
	_, ok := parse(version)
	return ok
}

func parse(tag string) (v [3]int, ok bool) {
	parts := strings.Split(strings.TrimPrefix(tag, "v"), ".")
	if len(parts) != 3 || !strings.HasPrefix(tag, "v") {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

// Executable returns the stable install path when running from a fish-named
// temporary copy, and the current executable path for direct developer runs.
func Executable() (string, error) {
	if exe := os.Getenv(InstallPathEnv); exe != "" {
		info, err := os.Stat(exe)
		if err != nil {
			return "", fmt.Errorf("inspect install executable: %w", err)
		}
		if info.IsDir() {
			return "", errors.New("install executable path is a directory")
		}
		return exe, nil
	}
	return os.Executable()
}

// Install puts the verified build at next in place of the executable exe.
// Windows keeps a running executable locked but lets it be renamed, so exe
// moves to exe.old, which Cleanup removes on the next start.
func Install(exe, next string) error {
	old := exe + ".old"
	_ = os.Remove(old) // left from an earlier update whose cleanup failed
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("move the running build aside: %w", err)
	}
	if err := os.Rename(next, exe); err != nil {
		_ = os.Rename(old, exe)
		return fmt.Errorf("put the new build in place: %w", err)
	}
	return nil
}

// Cleanup removes the build an earlier Install moved aside. It fails while
// that build still runs; the next start tries again.
func Cleanup(exe string) error {
	if err := os.Remove(exe + ".old"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Restart starts exe again with the same arguments and working directory.
func Restart(exe string) error {
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Env = CleanEnvironment(os.Environ())
	return cmd.Start()
}

// CleanEnvironment returns env without the launcher's variables, so a restart
// or a new fish-named copy starts from the installed executable's view.
func CleanEnvironment(env []string) []string {
	clean := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(key, InstallPathEnv) && !strings.EqualFold(key, FishChildEnv) && !strings.EqualFold(key, FishNameEnv) {
			clean = append(clean, entry)
		}
	}
	return clean
}
