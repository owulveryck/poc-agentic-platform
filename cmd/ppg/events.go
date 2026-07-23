package main

// Live observation of the decision-event journal (internal/journal): the
// validation server tails <state-root>/events.jsonl — written by ITSELF and
// by the other PPG processes (guards, MCP server, ppg-verify), each appending
// whole lines under flock — and re-serves it as Server-Sent Events, plus a
// small embedded dashboard.
//
// The routes are mounted OUTSIDE the reloadable corpus mux (see main): they
// depend only on the immutable journal path, so SIGHUP corpus reloads never
// touch a running stream.
//
// Tailing is stdlib-only polling. The reader never takes the writers' flock:
// writers append a whole line in a single write, so the only hazard is
// observing a torn final line — mitigated by never advancing the offset past
// the last '\n' seen.

import (
	"bytes"
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"
)

//go:embed dashboard.html
var dashboardHTML []byte

//go:embed loop.html
var loopHTML []byte

// streamDefaults for /events/stream. Poll is injectable for tests.
const (
	streamPollInterval = 500 * time.Millisecond
	streamPingEvery    = 15 * time.Second
	replayDefault      = 50
	replayMax          = 1000
	tailChunk          = 256 << 10 // bytes scanned backwards for ?replay
)

// tokensMarker is replaced by the content of design/tokens.css at serve
// time: the embedded page carries var(--color-*) references only (the
// design-token invariant forbids raw color values outside the tokens file),
// so the palette is injected from its canonical home instead of duplicated.
const tokensMarker = "/*DESIGN_TOKENS*/"

// servePage serves one embedded single-file view, injecting the design
// tokens read from tokensPath (best-effort: an absent palette degrades to
// the browser's defaults, it never fails the page). Shared by the table
// dashboard and the animated loop view.
func servePage(tokensPath string, page []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		out := page
		if tokens, err := os.ReadFile(tokensPath); err == nil {
			out = bytes.Replace(out, []byte(tokensMarker), tokens, 1)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(out)
	}
}

// handleEventStream tails the journal file and forwards each JSONL line as
// one SSE `data:` frame. On connect it replays the last ?replay=N complete
// lines (default 50, capped). Rotation (file renamed or truncated by
// journal.Open of another process) is detected as a size regression or a
// vanished file and restarts the tail from offset 0; lines written to the
// rotated-away file between the last poll and the rename are lost to the live
// stream (they remain available to `ppg report`).
func handleEventStream(path string, poll time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			httpError(w, http.StatusInternalServerError, map[string]any{"error": "streaming unsupported"})
			return
		}
		// The server sets a global WriteTimeout sized for request/response
		// exchanges; a stream must outlive it, so clear the deadline for
		// this connection only.
		_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		replay := replayDefault
		if v := r.URL.Query().Get("replay"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				replay = min(n, replayMax)
			}
		}
		lines, offset := tailLines(path, replay)
		for _, ln := range lines {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", ln)
		}
		flusher.Flush()

		ticker := time.NewTicker(poll)
		defer ticker.Stop()
		lastWrite := time.Now()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				newLines, newOffset, rotated := readNewLines(path, offset)
				if rotated {
					offset = 0
					continue // next tick re-reads the fresh file from the start
				}
				offset = newOffset
				for _, ln := range newLines {
					_, _ = fmt.Fprintf(w, "data: %s\n\n", ln)
				}
				if len(newLines) > 0 {
					flusher.Flush()
					lastWrite = time.Now()
				} else if time.Since(lastWrite) >= streamPingEvery {
					// SSE comment: keeps proxies and EventSource alive and
					// surfaces dead clients to the server.
					_, _ = fmt.Fprint(w, ": ping\n\n")
					flusher.Flush()
					lastWrite = time.Now()
				}
			}
		}
	}
}

// tailLines returns up to n last complete lines of the file and the absolute
// offset just past the last complete line (where the live tail resumes). A
// missing or empty file yields (nil, 0).
func tailLines(path string, n int) ([][]byte, int64) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil || st.Size() == 0 {
		return nil, 0
	}
	size := st.Size()
	start := max(size-tailChunk, 0)
	buf := make([]byte, size-start)
	if _, err := f.ReadAt(buf, start); err != nil {
		return nil, 0
	}
	if start > 0 {
		// The chunk starts mid-line: drop everything up to the first newline.
		i := bytes.IndexByte(buf, '\n')
		if i < 0 {
			return nil, size // one giant torn line: wait for its '\n'
		}
		start += int64(i + 1)
		buf = buf[i+1:]
	}
	last := bytes.LastIndexByte(buf, '\n')
	if last < 0 {
		return nil, start // no complete line yet
	}
	offset := start + int64(last+1)
	var lines [][]byte
	for _, ln := range bytes.Split(buf[:last], []byte{'\n'}) {
		if len(ln) > 0 {
			lines = append(lines, ln)
		}
	}
	if n == 0 {
		return nil, offset
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, offset
}

// readNewLines returns the complete lines appended since offset and the new
// offset. rotated is true when the file vanished or shrank below offset —
// the journal was rotated (or truncated) and the tail must restart at 0.
func readNewLines(path string, offset int64) (lines [][]byte, newOffset int64, rotated bool) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, offset, true
	}
	if st.Size() < offset {
		return nil, offset, true
	}
	if st.Size() == offset {
		return nil, offset, false
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, offset, false // transient: retry next tick
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, st.Size()-offset)
	if _, err := f.ReadAt(buf, offset); err != nil {
		return nil, offset, false
	}
	last := bytes.LastIndexByte(buf, '\n')
	if last < 0 {
		return nil, offset, false // torn line: wait for its '\n'
	}
	for _, ln := range bytes.Split(buf[:last], []byte{'\n'}) {
		if len(ln) > 0 {
			lines = append(lines, ln)
		}
	}
	return lines, offset + int64(last+1), false
}
