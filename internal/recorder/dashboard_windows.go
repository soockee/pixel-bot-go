package recorder

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/soocke/pixel-bot-go/internal/dataset"
)

//go:embed dashboard/index.html dashboard/dashboard.css dashboard/dashboard.js
var dashboardAssets embed.FS

const eventPageSize = 512

type eventPage struct {
	Events     []json.RawMessage `json:"events"`
	NextOffset int64             `json:"next_offset"`
	More       bool              `json:"more"`
	Live       bool              `json:"live"`
}

var errInvalidOffset = errors.New("offset must point to the start of a record")

func listenDashboard(address string) (net.Listener, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return nil, errors.New("dashboard address must use a loopback IP or localhost")
	}
	return net.Listen("tcp", net.JoinHostPort(host, port))
}

func startDashboard(listener net.Listener, trace *os.File, frames string, live bool, status io.Writer) func() {
	address := listener.Addr().String()
	server := &http.Server{
		Handler:           dashboardHandler(trace, frames, live, address),
		ReadHeaderTimeout: 3 * time.Second,
		WriteTimeout:      10 * time.Second,
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(status, "dashboard:", err)
		}
	}()
	fmt.Fprintf(status, "Dashboard: http://%s/\n", address)
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			_ = server.Close()
		}
		<-done
	}
}

func dashboardHandler(trace *os.File, frames string, live bool, authority string) http.Handler {
	static, err := fs.Sub(dashboardAssets, "dashboard")
	if err != nil {
		panic(err)
	}
	files := http.FileServerFS(static)
	mux := http.NewServeMux()
	mux.Handle("GET /{$}", files)
	mux.Handle("GET /dashboard.css", files)
	mux.Handle("GET /dashboard.js", files)
	mux.HandleFunc("GET /api/frame", func(w http.ResponseWriter, r *http.Request) {
		path := dataset.LatestFrame(frames)
		if path == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		http.ServeFile(w, r, path)
	})
	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		offset := int64(0)
		if raw := r.URL.Query().Get("offset"); raw != "" {
			var err error
			offset, err = strconv.ParseInt(raw, 10, 64)
			if err != nil || offset < 0 {
				http.Error(w, "invalid offset", http.StatusBadRequest)
				return
			}
		}
		page, err := readEventPage(trace, offset, live)
		if err != nil {
			code := http.StatusUnprocessableEntity
			if errors.Is(err, errInvalidOffset) {
				code = http.StatusBadRequest
			}
			http.Error(w, err.Error(), code)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(page)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != authority {
			http.Error(w, "use the local dashboard address printed by the recorder", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; frame-ancestors 'none'")
		mux.ServeHTTP(w, r)
	})
}

func readEventPage(trace *os.File, offset int64, live bool) (eventPage, error) {
	page := eventPage{Events: []json.RawMessage{}, NextOffset: offset, Live: live}
	info, err := trace.Stat()
	if err != nil {
		return page, fmt.Errorf("read trace metadata: %w", err)
	}
	size := info.Size()
	if offset < 0 || offset > size {
		return page, errInvalidOffset
	}
	if offset > 0 {
		var previous [1]byte
		if _, err := trace.ReadAt(previous[:], offset-1); err != nil {
			return page, fmt.Errorf("read trace offset: %w", err)
		}
		if previous[0] != '\n' {
			return page, errInvalidOffset
		}
	}
	reader := bufio.NewReaderSize(io.NewSectionReader(trace, offset, size-offset), 4096)
	for len(page.Events) < eventPageSize {
		line, err := reader.ReadSlice('\n')
		if errors.Is(err, io.EOF) {
			// A live writer can be in the middle of a line. Re-read it on the
			// next poll. A saved trace can end with a partial record as well.
			break
		}
		if err != nil {
			return page, fmt.Errorf("read trace record: %w", err)
		}
		value := bytes.TrimSpace(line)
		if !json.Valid(value) || value[0] != '{' {
			return page, fmt.Errorf("invalid JSON record at byte %d", page.NextOffset)
		}
		page.Events = append(page.Events, json.RawMessage(bytes.Clone(value)))
		page.NextOffset += int64(len(line))
	}
	page.More = len(page.Events) == eventPageSize && page.NextOffset < size
	return page, nil
}

func viewTrace(ctx context.Context, listener net.Listener, path, frames string, status io.Writer) error {
	trace, err := os.Open(path)
	if err != nil {
		return err
	}
	defer trace.Close()
	info, err := trace.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("saved trace must be a regular file")
	}
	stop := startDashboard(listener, trace, frames, false, status)
	defer stop()
	<-ctx.Done()
	return nil
}
