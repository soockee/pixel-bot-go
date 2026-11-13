package main

import (
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/soocke/pixel-bot-go/internal/update"
)

var fishSpecies = [...]string{
	"anchovy", "anglerfish", "arowana", "barracuda", "betta", "bluegill",
	"blobfish", "catfish", "clownfish", "cod", "cowfish", "discus",
	"flounder", "frogfish", "goldfish", "guppy", "haddock", "halibut",
	"handfish", "koi", "lionfish", "loach", "lumpsucker", "mahi-mahi",
	"mackerel", "minnow", "molly", "mudskipper", "oscar", "parrotfish",
	"perch", "pike", "piranha", "pufferfish", "rainbowfish", "salmon",
	"sardine", "seahorse", "snapper", "sturgeon", "swordfish", "tetra",
	"tilapia", "trout", "tuna", "wrasse", "zebrafish",
}

func launchFishProcess() (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 1, err
	}
	cleanupFishCopies(os.TempDir())
	process, _, err := startFishCopy(exe, os.Args[1:])
	if err != nil {
		return 1, err
	}
	if len(os.Args) == 1 {
		_ = process.Release()
		return 0, nil
	}
	// Runs with flags come from scripts, which need the child's exit code.
	// Ctrl+C reaches the child on the shared console; wait for it to finish.
	signal.Ignore(os.Interrupt)
	state, err := process.Wait()
	if err != nil {
		// The copy is running, so an error here must not make main start a second app.
		fmt.Fprintln(os.Stderr, "wait for fish-named app:", err)
		return 1, nil
	}
	return state.ExitCode(), nil
}

func startFishCopy(exe string, args []string) (*os.Process, string, error) {
	exe, err := filepath.Abs(exe)
	if err != nil {
		return nil, "", err
	}
	dir, err := os.MkdirTemp("", "pixel-bot-go-fish-")
	if err != nil {
		return nil, "", fmt.Errorf("create temporary launch folder: %w", err)
	}
	started := false
	defer func() {
		if !started {
			_ = os.RemoveAll(dir)
		}
	}()

	species := fishSpecies[rand.IntN(len(fishSpecies))]
	fishExe := filepath.Join(dir, species+".exe")
	if err := copyExecutable(exe, fishExe); err != nil {
		return nil, "", err
	}
	cmd := exec.Command(fishExe, args...)
	cmd.Env = childEnvironment(exe, species)
	if len(args) > 0 {
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	}
	if err := cmd.Start(); err != nil {
		return nil, "", fmt.Errorf("start fish-named app: %w", err)
	}
	started = true
	return cmd.Process, dir, nil
}

func cleanupFishCopies(root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "pixel-bot-go-fish-") {
			continue
		}
		info, err := entry.Info()
		// Don't disturb another copy that was launched recently.
		if err == nil && info.ModTime().Before(cutoff) {
			_ = os.RemoveAll(filepath.Join(root, entry.Name()))
		}
	}
}

func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open app executable: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		return fmt.Errorf("create fish-named app executable: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("copy app executable: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("finish fish-named app executable: %w", err)
	}
	return nil
}

func childEnvironment(exe, species string) []string {
	return append(update.CleanEnvironment(os.Environ()), update.InstallPathEnv+"="+exe, update.FishChildEnv+"=1", update.FishNameEnv+"="+species)
}
