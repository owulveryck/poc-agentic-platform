package main

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// appendLine appends one JSONL line to the journal file like a PPG writer
// would (whole line, single write).
func appendLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatalf("append: %v", err)
	}
}

// sseClient connects to the stream and returns a function yielding the next
// data frame (skipping SSE comments), with a per-read timeout.
func sseClient(t *testing.T, url string) func() string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	r := bufio.NewReader(resp.Body)
	return func() string {
		deadline := time.After(5 * time.Second)
		lines := make(chan string, 1)
		errs := make(chan error, 1)
		go func() {
			for {
				line, err := r.ReadString('\n')
				if err != nil {
					errs <- err
					return
				}
				line = strings.TrimRight(line, "\n")
				if data, ok := strings.CutPrefix(line, "data: "); ok {
					lines <- data
					return
				}
			}
		}()
		select {
		case l := <-lines:
			return l
		case err := <-errs:
			t.Fatalf("reading stream: %v", err)
		case <-deadline:
			t.Fatal("timed out waiting for an SSE data frame")
		}
		return ""
	}
}

func TestEventStreamReplayAndLiveTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	appendLine(t, path, `{"name":"ppg.test.1"}`)
	appendLine(t, path, `{"name":"ppg.test.2"}`)
	appendLine(t, path, `{"name":"ppg.test.3"}`)

	srv := httptest.NewServer(handleEventStream(path, 10*time.Millisecond))
	t.Cleanup(srv.Close)

	next := sseClient(t, srv.URL+"?replay=2")
	// Replay: only the last 2 of the 3 seeded lines.
	if got := next(); got != `{"name":"ppg.test.2"}` {
		t.Fatalf("first replayed frame = %s, want ppg.test.2", got)
	}
	if got := next(); got != `{"name":"ppg.test.3"}` {
		t.Fatalf("second replayed frame = %s, want ppg.test.3", got)
	}

	// Live tail: a line appended after connect streams through.
	appendLine(t, path, `{"name":"ppg.test.4"}`)
	if got := next(); got != `{"name":"ppg.test.4"}` {
		t.Fatalf("live frame = %s, want ppg.test.4", got)
	}
}

func TestEventStreamRecoversFromRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	appendLine(t, path, `{"name":"ppg.old.1"}`)

	srv := httptest.NewServer(handleEventStream(path, 10*time.Millisecond))
	t.Cleanup(srv.Close)

	next := sseClient(t, srv.URL+"?replay=10")
	if got := next(); got != `{"name":"ppg.old.1"}` {
		t.Fatalf("replayed frame = %s, want ppg.old.1", got)
	}

	// Rotate like journal.Open does: rename, then a fresh file appears.
	if err := os.Rename(path, filepath.Join(dir, "events-1.jsonl")); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	// Give the poller a tick to observe the vanished file before the fresh
	// one appears, then write into the new journal.
	time.Sleep(30 * time.Millisecond)
	appendLine(t, path, `{"name":"ppg.new.1"}`)
	if got := next(); got != `{"name":"ppg.new.1"}` {
		t.Fatalf("post-rotation frame = %s, want ppg.new.1", got)
	}
}

func TestTailLinesTornLineIsHeldBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	// One complete line, then a torn one (no trailing newline).
	if err := os.WriteFile(path, []byte("{\"a\":1}\n{\"torn\":"), 0o600); err != nil {
		t.Fatal(err)
	}
	lines, offset := tailLines(path, 10)
	if len(lines) != 1 || string(lines[0]) != `{"a":1}` {
		t.Fatalf("lines = %q, want the single complete line", lines)
	}
	// The offset must sit right after the complete line so the torn tail is
	// re-read once its newline lands.
	if want := int64(len("{\"a\":1}\n")); offset != want {
		t.Fatalf("offset = %d, want %d", offset, want)
	}
}

// TestLoopViewIsServedWithTokens proves the animated loop view is served
// with the design tokens injected in place of the marker and the diagram
// anchors present (the SVG boxes the event animation lights up).
func TestLoopViewIsServedWithTokens(t *testing.T) {
	dir := t.TempDir()
	tokens := filepath.Join(dir, "tokens.css")
	if err := os.WriteFile(tokens, []byte(":root { --color-bg: token-value; }"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(servePage(tokens, loopHTML))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body := new(strings.Builder)
	if _, err := io.Copy(body, resp.Body); err != nil {
		t.Fatal(err)
	}
	page := body.String()
	if !strings.Contains(page, "token-value") {
		t.Fatal("design tokens must be injected in place of the marker")
	}
	if strings.Contains(page, "/*DESIGN_TOKENS*/") {
		t.Fatal("the tokens marker must be replaced")
	}
	for _, anchor := range []string{
		`id="b-capture"`, `id="b-plan"`, `id="b-act"`, `id="b-observe"`,
		`id="c-mcp"`, `id="c-guard"`, `id="c-corpus"`, `id="nbadge-text"`,
		"Replay this session",
		// v2: single-session trace + theme
		`id="badges"`, `id="traces"`, `id="tooltip"`, `id="themebtn"`, "theme-light",
		// v2.1: guides share the motion-path geometry; MCP vs hook semantics
		`id="guides"`, "not MCP",
		// v2.2: full-record modal in the loop view too
		`id="modal-layer"`, `id="m-request"`, `id="m-reply"`,
	} {
		if !strings.Contains(page, anchor) {
			t.Fatalf("loop view must contain %s", anchor)
		}
	}
}
