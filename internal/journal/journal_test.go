package journal

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEmitConcurrent(t *testing.T) {
	root := t.TempDir()
	const writers, perWriter = 50, 20

	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			// Separate Writer instances on the same file: the multi-process
			// scenario (server + guards + verify sharing the state root).
			w := Open(root, "test", "/proj")
			for j := range perWriter {
				w.Emit(Event{
					Name:      "ppg.test.event",
					SessionID: "sess",
					Attrs:     map[string]any{"writer": n, "seq": j, "pad": strings.Repeat("x", 512)},
				})
			}
		}(i)
	}
	wg.Wait()

	f, err := os.Open(filepath.Join(root, FileName))
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024)
	count := 0
	for sc.Scan() {
		var e Event
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("line %d does not parse (interleaved write?): %v", count+1, err)
		}
		if e.Name != "ppg.test.event" || e.Component != "test" || e.Project != "/proj" {
			t.Fatalf("line %d has wrong envelope: %+v", count+1, e)
		}
		count++
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if count != writers*perWriter {
		t.Fatalf("got %d lines, want %d", count, writers*perWriter)
	}
}

func TestEmitDefaults(t *testing.T) {
	root := t.TempDir()
	w := Open(root, "comp", "")
	before := time.Now().UTC()
	w.Emit(Event{Name: "ppg.x"})

	raw, err := os.ReadFile(filepath.Join(root, FileName))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var e Event
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if e.Severity != SeverityInfo {
		t.Errorf("severity = %q, want INFO default", e.Severity)
	}
	if e.Time.Before(before.Add(-time.Second)) {
		t.Errorf("time not stamped: %v", e.Time)
	}
	if e.Component != "comp" {
		t.Errorf("component = %q", e.Component)
	}
}

func TestNilWriterNoop(t *testing.T) {
	var w *Writer
	w.Emit(Event{Name: "ppg.x"}) // must not panic
}

func TestKillSwitch(t *testing.T) {
	for _, v := range []string{"off", "0", "false"} {
		t.Setenv(EnvDisable, v)
		root := t.TempDir()
		w := Open(root, "c", "")
		if w != nil {
			t.Fatalf("PPG_TELEMETRY=%s: Open returned non-nil", v)
		}
		w.Emit(Event{Name: "ppg.x"})
		if _, err := os.Stat(filepath.Join(root, FileName)); !os.IsNotExist(err) {
			t.Fatalf("PPG_TELEMETRY=%s: journal file was created", v)
		}
	}
}

func TestOpenEmptyRoot(t *testing.T) {
	if w := Open("", "c", ""); w != nil {
		t.Fatal("Open with empty root returned non-nil")
	}
}

func TestRotationAtOpen(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, FileName)
	big := make([]byte, rotateBytes)
	for i := range big {
		big[i] = 'x'
	}
	if err := os.WriteFile(path, big, 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := Open(root, "c", "")
	w.Emit(Event{Name: "ppg.x"})

	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("journal missing after rotation: %v", err)
	}
	if st.Size() >= rotateBytes {
		t.Fatalf("journal was not rotated (size %d)", st.Size())
	}
	rotated, err := filepath.Glob(filepath.Join(root, "events-*.jsonl"))
	if err != nil || len(rotated) != 1 {
		t.Fatalf("rotated file: got %v (err %v), want exactly one", rotated, err)
	}
}
