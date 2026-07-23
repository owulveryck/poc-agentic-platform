// Package journal is the decision-event telemetry sink shared by every PPG
// component (validation server, guards, MCP server, ppg-verify). It appends
// wide events — one self-contained JSONL record per governance decision or
// lifecycle moment — to a single per-machine file, for a-posteriori analysis
// of what the guardrails blocked, allowed, or missed (jq/duckdb over
// <state-root>/events.jsonl).
//
// The event shape mirrors the OpenTelemetry Logs data model so a future OTLP
// exporter is a pure field-for-field adapter, never a schema migration:
//
//	Time      -> LogRecord.Timestamp
//	Name      -> LogRecord.EventName (namespaced "ppg.*")
//	Severity  -> LogRecord.SeverityText (INFO | WARN | ERROR)
//	Component -> Resource attribute service.name (which binary emitted it)
//	SessionID -> promoted attribute session.id — THE correlation key across
//	             server, guards, MCP server and ppg-verify events
//	Project   -> promoted attribute; the normalized project dir for
//	             project-scoped emitters (guards, verify, mcp), empty from
//	             the server, which is project-agnostic
//	Attrs     -> LogRecord.Attributes (dotted, semconv-style names)
//
// Privacy contract: events carry paths, hashes, policy ids, byte counts and
// the plan intent — NEVER file contents, edit payloads, or violation message
// bodies (those may quote file content). The intent is already persisted in
// escalations.jsonl, so it introduces no new exposure class.
//
// Emission is best-effort by design: a telemetry failure is logged to stderr
// and never fails, delays, or alters the caller's decision — the same
// contract as the escalation log.
package journal

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// EnvDisable is the kill switch: PPG_TELEMETRY=off|0|false disables
// emission entirely (Open returns nil, no file is ever created).
const EnvDisable = "PPG_TELEMETRY"

// FileName is the journal file, a sibling of escalations.jsonl under the
// store root.
const FileName = "events.jsonl"

// rotateBytes is the size past which Open renames the current journal to
// events-<unix-ts>.jsonl before appending to a fresh one. Rotation only
// happens at Open (process start), so a long-lived server may overshoot;
// that is an accepted trade-off for having no daemon and no write-path cost.
const rotateBytes = 64 << 20 // 64 MiB

// Severity values. INFO = allow/OK, WARN = policy denial, ERROR =
// infra failure / fail-closed.
const (
	SeverityInfo  = "INFO"
	SeverityWarn  = "WARN"
	SeverityError = "ERROR"
)

// Event is one wide decision or lifecycle record. The four promoted fields
// besides Time are exactly the columns every query groups or filters on;
// everything else goes in Attrs so adding a field never breaks the schema.
type Event struct {
	Time      time.Time      `json:"time"`                 // RFC3339Nano UTC; set by Emit when zero
	Name      string         `json:"name"`                 // "ppg.*" event name
	Severity  string         `json:"severity,omitempty"`   // defaults to INFO in Emit
	Component string         `json:"component"`            // set by the Writer
	SessionID string         `json:"session_id,omitempty"` // correlation key
	Project   string         `json:"project,omitempty"`    // set by the Writer
	Attrs     map[string]any `json:"attributes,omitempty"`
}

// Writer appends events to a single JSONL file shared by multiple processes.
// A nil *Writer is valid and inert: Emit on nil is a no-op, so callers never
// nil-check (mirrors how a disabled journal behaves).
type Writer struct {
	path      string
	component string
	project   string
}

// Disabled reports whether the kill switch turns telemetry off.
func Disabled() bool {
	switch os.Getenv(EnvDisable) {
	case "off", "0", "false":
		return true
	}
	return false
}

// Open returns a Writer appending to <stateRoot>/events.jsonl, or nil when
// telemetry is disabled or stateRoot is empty. component names the emitting
// binary (e.g. "ppg", "ppg-guard"); project is the normalized project dir
// for project-scoped emitters, "" otherwise. Open never fails: a rotation
// error is logged and ignored (the file keeps growing until the next
// successful rotation).
func Open(stateRoot, component, project string) *Writer {
	if stateRoot == "" || Disabled() {
		return nil
	}
	w := &Writer{
		path:      filepath.Join(stateRoot, FileName),
		component: component,
		project:   project,
	}
	w.rotateIfOversized()
	return w
}

// Emit appends one event as a single JSONL line under an exclusive flock.
// Failures are logged and swallowed — telemetry must never fail or block
// the decision being recorded. Emit stamps Time (UTC now), Severity (INFO),
// Component and Project when the caller left them zero.
func (w *Writer) Emit(e Event) {
	if w == nil {
		return
	}
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	if e.Severity == "" {
		e.Severity = SeverityInfo
	}
	e.Component = w.component
	if e.Project == "" {
		e.Project = w.project
	}
	line, err := json.Marshal(e)
	if err != nil {
		log.Printf("journal: marshal %s: %v", e.Name, err)
		return
	}
	line = append(line, '\n')
	if err := w.append(line); err != nil {
		log.Printf("journal: %v", err)
	}
}

// append opens the journal, takes an exclusive flock, and writes the
// pre-marshaled line in a single Write call. The flock serializes the five
// PPG processes that share the file; it is released on close (and by the
// kernel if the process dies), so a crashed emitter never wedges the file.
func (w *Writer) append(line []byte) error {
	if err := os.MkdirAll(filepath.Dir(w.path), 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(w.path), err)
	}
	f, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open %s: %w", w.path, err)
	}
	defer func() { _ = f.Close() }()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("flock %s: %w", w.path, err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()
	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("write %s: %w", w.path, err)
	}
	return nil
}

// rotateIfOversized renames the journal to events-<unix-ts>.jsonl when it
// exceeds rotateBytes. Racing processes may both attempt the rename; the
// loser's error is ignored (the winner already moved the file). Rotated
// files are plain JSONL the operator can archive or delete.
func (w *Writer) rotateIfOversized() {
	st, err := os.Stat(w.path)
	if err != nil || st.Size() < rotateBytes {
		return
	}
	rotated := filepath.Join(filepath.Dir(w.path),
		fmt.Sprintf("events-%d.jsonl", time.Now().UTC().Unix()))
	if err := os.Rename(w.path, rotated); err != nil && !os.IsNotExist(err) {
		log.Printf("journal: rotate: %v", err)
	}
}
