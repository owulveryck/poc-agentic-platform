// Command ppg-guard is a Claude Code hook binary serving two events:
//
//   - SessionStart: records the real session id in the SessionStore and
//     purges any leftover tickets in the TokenStore, so a capability never
//     survives the session that locked it.
//   - PreToolUse: verifies every file-mutating tool call against the
//     capability ticket locked through the validation server —
//     signature, TTL, path scope, session binding — AND the actual edited
//     content against the artifact-view policy corpus (via POST
//     /verify_artifact on the validation server).
//
// Contract (see https://code.claude.com/docs/en/hooks): the hook receives a
// JSON payload on stdin; exit code 2 blocks the tool call and stderr is fed
// back to the model — which turns this hook into the deterministic in-tool
// guard of the amplified loop, running inside an off-the-shelf agent.
//
// The guard fails CLOSED: if it cannot evaluate an edit (unreadable payload,
// unopenable store, unreachable validation server), it blocks rather than letting the
// edit through. SessionStart, which is not a security gate, never blocks.
//
// Storage lives per-machine under $XDG_STATE_HOME/ppg/projects/<slug>/
// (see internal/store). The project is resolved from --project-dir >
// PPG_PROJECT_DIR > the hook payload's cwd > os.Getwd(). The validation server base URL
// is PPG_URL (default http://localhost:8765).
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/owulveryck/poc-agentic-platform/internal/smarttools"
	"github.com/owulveryck/poc-agentic-platform/internal/store"
	"github.com/owulveryck/poc-agentic-platform/internal/ticket"
	"github.com/owulveryck/poc-agentic-platform/internal/version"
)

// hookInput is the subset of the hook payloads the guard needs. Content and
// path may arrive under several field names depending on the tool.
type hookInput struct {
	HookEventName string `json:"hook_event_name"`
	ToolName      string `json:"tool_name"`
	SessionID     string `json:"session_id"`
	CWD           string `json:"cwd"`
	ToolInput     struct {
		FilePath     string `json:"file_path"`
		Path         string `json:"path"`
		NotebookPath string `json:"notebook_path"`
		NewString    string `json:"new_string"`
		NewStr       string `json:"new_str"`
		Content      string `json:"content"`
	} `json:"tool_input"`
}

// artifactVerifier reports the architectural-invariant violation messages of a
// file's proposed content, or an error when the check could not run (which the
// caller treats as fail-closed). Injected so decide stays offline-testable.
type artifactVerifier func(ticket, path, content string) (violations []string, err error)

func main() {
	projectDirFlag := flag.String("project-dir", "",
		"absolute project directory (overrides "+store.EnvProjectDir+" and payload cwd)")
	storeRootFlag := flag.String("store-root", "",
		"per-machine state root (overrides "+store.EnvStoreRoot+"); defaults to $XDG_STATE_HOME/ppg")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("ppg-guard " + version.String())
		return
	}

	payload, err := os.ReadFile("/dev/stdin")
	if err != nil {
		// Without a payload we cannot even tell PreToolUse from SessionStart;
		// this is an OS-level failure, not an agent edit slipping through.
		fmt.Fprintf(os.Stderr, "ppg-guard: cannot read hook payload: %v\n", err)
		os.Exit(1)
	}
	var in hookInput
	if err := json.Unmarshal(payload, &in); err != nil {
		// A payload we cannot parse could be a write about to happen: treat it
		// as PreToolUse and fail closed rather than proceeding on zero values.
		failInfra(true, "malformed hook payload: "+err.Error())
	}
	// Some harnesses omit hook_event_name on PreToolUse; a write tool implies it.
	isPreTool := in.HookEventName == "PreToolUse" || (in.HookEventName != "SessionStart" && isWriteTool(in.ToolName))

	root, err := store.ResolveRoot(*storeRootFlag)
	if err != nil {
		failInfra(isPreTool, "cannot resolve state root: "+err.Error())
	}
	// The guard verifies the ticket signature locally: it needs the same
	// per-machine signing key as the validation server ($PPG_TICKET_SECRET wins).
	if err := ticket.UseKeyFile(filepath.Join(root, "ticket.key")); err != nil {
		failInfra(isPreTool, "cannot load ticket signing key: "+err.Error())
	}
	projectDir, err := store.ResolveProjectDir(*projectDirFlag, projectDirFallback(in))
	if err != nil {
		failInfra(isPreTool, "cannot resolve project dir: "+err.Error())
	}
	st, err := store.NewFilesystem(root, projectDir)
	if err != nil {
		failInfra(isPreTool, "cannot open store: "+err.Error())
	}

	if in.HookEventName == "SessionStart" {
		if err := recordSession(in, st, st); err != nil {
			fmt.Fprintf(os.Stderr, "ppg-guard: cannot record session: %v\n", err)
		}
		os.Exit(0) // SessionStart never blocks
	}

	verify := func(ticket, path, content string) ([]string, error) {
		return verifyArtifactRemote(gatewayURL(), ticket, path, content)
	}
	block, msg := decide(payload, readTicket(in, st, st), verify)
	if block {
		fmt.Fprintln(os.Stderr, msg)
		os.Exit(2) // blocking: stderr goes back to the model
	}
	os.Exit(0) // no decision: normal permission flow applies
}

// failInfra blocks (fail-closed) when the guard cannot evaluate a PreToolUse
// edit; for other events an infrastructure error is logged but not blocking.
func failInfra(isPreTool bool, msg string) {
	if isPreTool {
		fmt.Fprintln(os.Stderr, "PPG_GUARD_ERROR: "+msg+
			" — blocking (fail-closed): the guard cannot verify this edit. "+
			"Fix the validation server/state setup, or re-lock your plan, and retry.")
		os.Exit(2)
	}
	fmt.Fprintln(os.Stderr, "ppg-guard: "+msg)
	os.Exit(0)
}

// projectDirFallback returns the payload's cwd if present, otherwise the
// process cwd. It is only used when neither flag nor env resolves the
// project dir; the returned value can still be "" if os.Getwd fails.
func projectDirFallback(in hookInput) string {
	if in.CWD != "" {
		return in.CWD
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

// recordSession persists the session id for the MCP server and purges any
// ticket inherited from a previous session: the capability dies with the
// session that locked it, not only with its TTL (8h by default — see
// internal/ticket.DefaultTTL).
func recordSession(in hookInput, ts store.TokenStore, ss store.SessionStore) error {
	if in.SessionID == "" {
		return nil
	}
	if err := ts.Reset(); err != nil {
		return err
	}
	return ss.PutActive(in.SessionID)
}

// readTicket loads the capability ticket for the session id carried by the
// hook payload; falls back to the store's active session when the payload
// omits it (older harness). Returns "" when no ticket is available.
func readTicket(in hookInput, ts store.TokenStore, ss store.SessionStore) string {
	sid := in.SessionID
	if sid == "" {
		active, err := ss.GetActive()
		if err != nil {
			return ""
		}
		sid = active
	}
	if sid == "" {
		return ""
	}
	tok, err := ts.Get(sid)
	if err != nil {
		return ""
	}
	return tok
}

// isWriteTool reports whether a tool mutates files and must therefore be
// guarded. It covers the Claude Code write tools by name (including
// NotebookEdit, whose path lives in notebook_path) plus a defensive superset
// for future/variant names that advertise editing or writing.
func isWriteTool(name string) bool {
	switch name {
	case "Edit", "Write", "MultiEdit", "NotebookEdit", "Update",
		"create_file", "edit_file", "editFiles", "str_replace_editor", "apply_patch":
		return true
	}
	return strings.Contains(name, "Edit") || strings.Contains(name, "Write")
}

// targetPath returns the file path from whichever field the tool used.
func targetPath(in hookInput) string {
	switch {
	case in.ToolInput.FilePath != "":
		return in.ToolInput.FilePath
	case in.ToolInput.Path != "":
		return in.ToolInput.Path
	case in.ToolInput.NotebookPath != "":
		return in.ToolInput.NotebookPath
	}
	return ""
}

// editedContent returns the proposed content from whichever field the tool used.
func editedContent(in hookInput) string {
	switch {
	case in.ToolInput.NewString != "":
		return in.ToolInput.NewString
	case in.ToolInput.NewStr != "":
		return in.ToolInput.NewStr
	case in.ToolInput.Content != "":
		return in.ToolInput.Content
	}
	return ""
}

// decide is the decision function: given the hook payload, the raw ticket, and
// an artifact verifier, it returns whether to block and the semantic message
// for the model. It gates on the tool name (not merely the presence of a path),
// checks path scope and session binding locally, then verifies the edited
// content against the artifact-view policy through verify. A nil verifier skips
// the content step (used by offline tests).
func decide(payload []byte, rawTicket string, verify artifactVerifier) (bool, string) {
	var in hookInput
	if err := json.Unmarshal(payload, &in); err != nil {
		return true, "PPG_GUARD_ERROR: unreadable hook payload; blocking (fail-closed)."
	}
	if !isWriteTool(in.ToolName) {
		return false, "" // not a file-mutating tool: nothing to guard
	}
	target := targetPath(in)
	if target == "" {
		return true, "PPG_GUARD_ERROR: " + in.ToolName +
			" is a file-mutating tool but no target path was found in tool_input; blocking (fail-closed)."
	}
	if smarttools.IsHarnessMetadata(target) {
		return false, "" // harness plan-file bookkeeping, never in ticket scope
	}

	if rawTicket == "" {
		return true, "No capability ticket for this session. " +
			"Lock a plan first: call the lock_in_plan tool (or POST /lock_in_plan on the " +
			"validation server) — the returned execution_ticket is persisted for you."
	}

	rel := relativeTarget(target, in.CWD)
	claims, err := smarttools.GuardTargets(rawTicket, []string{rel})
	if err != nil {
		var oos *smarttools.OutOfScopeError
		if errors.As(err, &oos) {
			return true, fmt.Sprintf(
				"OUT_OF_PLAN_SCOPE: %q is not part of the locked plan (allowed: %s). "+
					"Nothing was modified. If this change is genuinely needed, re-plan through lock_in_plan.",
				oos.Attempted, strings.Join(oos.Allowed, ", "))
		}
		return true, "Capability ticket rejected: " + err.Error() +
			". Re-lock your plan through lock_in_plan."
	}
	if in.SessionID != "" && claims.SessionID != in.SessionID {
		return true, fmt.Sprintf(
			"SESSION_MISMATCH: the capability ticket was issued for session %q, not for this session (%q). "+
				"A ticket dies with the session that locked it. Nothing was modified: re-plan through lock_in_plan.",
			claims.SessionID, in.SessionID)
	}

	// Content policy (artifact view): the path is in scope, but the bytes must
	// also satisfy the invariants. Always consult the policy — even an empty
	// or unrecognized content payload can violate a path-scoped content rule
	// (e.g. a governed file that must not be touched in-session at all), so
	// skipping on empty content would be a hole, not an optimization. Fail
	// closed if the check cannot run.
	if verify != nil {
		violations, err := verify(rawTicket, rel, editedContent(in))
		if err != nil {
			return true, "PPG_GUARD_ERROR: cannot verify content against policy: " + err.Error() +
				" — blocking (fail-closed). Nothing was modified."
		}
		if len(violations) > 0 {
			return true, "ARCHITECTURAL_INVARIANT_VIOLATION: " + strings.Join(violations, " | ") +
				" Nothing was modified; fix the content to satisfy the invariant and resubmit."
		}
	}
	return false, ""
}

// relativeTarget converts the absolute file path Claude Code passes into the
// project-relative path the plan scope is expressed in. It cleans the result so
// a "../" cannot escape scope through the fallback branch.
func relativeTarget(filePath, cwd string) string {
	if cwd == "" || !filepath.IsAbs(filePath) {
		return filepath.ToSlash(filepath.Clean(filePath))
	}
	rel, err := filepath.Rel(cwd, filePath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(filepath.Clean(filePath))
	}
	return filepath.ToSlash(rel)
}

// gatewayURL is the validation server base URL (PPG_URL, default
// http://localhost:8765) — the same convention as the MCP server.
func gatewayURL() string {
	if u := os.Getenv("PPG_URL"); u != "" {
		return u
	}
	return "http://localhost:8765"
}

var httpClient = &http.Client{Timeout: 5 * time.Second}

// verifyArtifactRemote asks the validation server to evaluate the artifact-view policy
// against the edited content. A transport error is returned (fail-closed);
// a policy rejection is returned as violation messages.
func verifyArtifactRemote(gateway, ticket, path, content string) ([]string, error) {
	body, _ := json.Marshal(map[string]string{"ticket": ticket, "path": path, "content": content})
	resp, err := httpClient.Post(strings.TrimRight(gateway, "/")+"/verify_artifact",
		"application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var out struct {
		Status     string `json:"status"`
		Violations []struct {
			Message string `json:"message"`
		} `json:"violations"`
		Guidance string `json:"guidance"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding validation server response: %w", err)
	}
	switch out.Status {
	case "ARTIFACT_OK":
		return nil, nil
	case "ARTIFACT_REJECTED", "REFUSED":
		var msgs []string
		for _, v := range out.Violations {
			msgs = append(msgs, v.Message)
		}
		if len(msgs) == 0 && out.Guidance != "" {
			msgs = append(msgs, out.Guidance)
		}
		return msgs, nil
	default:
		return nil, fmt.Errorf("unexpected validation server status %q (HTTP %d)", out.Status, resp.StatusCode)
	}
}
