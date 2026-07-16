// Command ppg-copilot-guard is a GitHub Copilot pre-tool hook binary that
// mirrors adapters/claudecode/guard for the Copilot hook contract.
//
// Two events are served through the same binary:
//
//   - SessionStart: records the real session id in the SessionStore and
//     purges any leftover tickets in the TokenStore, so a capability
//     never survives the session that locked it.
//   - PreToolUse: verifies every file-mutating tool call against the
//     capability ticket locked through the Platform Planning Gateway —
//     signature, TTL, path scope, session binding — AND the actual edited
//     content against the artifact-view policy corpus (via POST
//     /verify_artifact on the gateway).
//
// The hook receives a JSON payload on stdin. Unlike Claude Code (which
// reads the exit code and stderr), Copilot expects a JSON decision on
// stdout — see https://code.visualstudio.com/docs/agents/reference/hooks-reference.
// The decision logic is otherwise identical to the Claude guard.
//
// The guard fails CLOSED: if it cannot evaluate an edit (unopenable store,
// unreachable gateway), it denies rather than allowing. SessionStart, which is
// not a security gate, always allows.
//
// Storage lives per-machine under $XDG_STATE_HOME/ppg/projects/<slug>/
// (see internal/store). The project is resolved from --project-dir >
// PPG_PROJECT_DIR > the hook payload's cwd > os.Getwd(). The gateway base URL
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
)

// hookInput is the subset of the Copilot hook payload the guard needs. The
// Copilot desktop app names the file path field `path`; a few tools use
// `file_path` (and NotebookEdit-style tools `notebook_path`). Content arrives
// as `new_str`, `new_string`, or `content`. Accept all.
type hookInput struct {
	HookEventName string `json:"hook_event_name"`
	ToolName      string `json:"tool_name"`
	SessionID     string `json:"session_id"`
	CWD           string `json:"cwd"`
	ToolInput     struct {
		Path         string `json:"path"`
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
		NewStr       string `json:"new_str"`
		NewString    string `json:"new_string"`
		Content      string `json:"content"`
	} `json:"tool_input"`
}

// targetPath returns whichever known file-path field is set.
func (h hookInput) targetPath() string {
	switch {
	case h.ToolInput.Path != "":
		return h.ToolInput.Path
	case h.ToolInput.FilePath != "":
		return h.ToolInput.FilePath
	case h.ToolInput.NotebookPath != "":
		return h.ToolInput.NotebookPath
	}
	return ""
}

// editedContent returns the proposed content from whichever field the tool used.
func (h hookInput) editedContent() string {
	switch {
	case h.ToolInput.NewStr != "":
		return h.ToolInput.NewStr
	case h.ToolInput.NewString != "":
		return h.ToolInput.NewString
	case h.ToolInput.Content != "":
		return h.ToolInput.Content
	}
	return ""
}

// isWriteTool reports whether the tool mutates files and must be guarded. It
// covers Copilot's Edit/Write, the VS Code Copilot Chat `editFiles`, and a
// defensive superset for variant names — kept identical to the Claude guard so
// the two adapters never diverge on which tools they gate.
func isWriteTool(name string) bool {
	switch name {
	case "Edit", "Write", "MultiEdit", "NotebookEdit", "Update",
		"create_file", "edit_file", "editFiles", "str_replace_editor", "apply_patch":
		return true
	}
	return strings.Contains(name, "Edit") || strings.Contains(name, "Write")
}

// artifactVerifier reports the architectural-invariant violation messages of a
// file's proposed content, or an error when the check could not run.
type artifactVerifier func(ticket, path, content string) (violations []string, err error)

func main() {
	projectDirFlag := flag.String("project-dir", "",
		"absolute project directory (overrides "+store.EnvProjectDir+" and payload cwd)")
	storeRootFlag := flag.String("store-root", "",
		"per-machine state root (overrides "+store.EnvStoreRoot+"); defaults to $XDG_STATE_HOME/ppg")
	flag.Parse()

	payload, err := os.ReadFile("/dev/stdin")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ppg-copilot-guard: cannot read hook payload: %v\n", err)
		os.Exit(1)
	}
	var in hookInput
	_ = json.Unmarshal(payload, &in)
	isPreTool := in.HookEventName == "PreToolUse" || (in.HookEventName != "SessionStart" && isWriteTool(in.ToolName))

	root, err := store.ResolveRoot(*storeRootFlag)
	if err != nil {
		failInfra(isPreTool, "cannot resolve state root: "+err.Error())
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
			fmt.Fprintf(os.Stderr, "ppg-copilot-guard: cannot record session: %v\n", err)
		}
		emitAllow()
		return
	}

	verify := func(ticket, path, content string) ([]string, error) {
		return verifyArtifactRemote(gatewayURL(), ticket, path, content)
	}
	block, msg := decide(payload, readTicket(in, st, st), verify)
	if block {
		emitDeny(msg)
		return
	}
	emitAllow()
}

// failInfra denies (fail-closed) when the guard cannot evaluate a PreToolUse
// edit; for other events an infrastructure error is logged but allowed.
func failInfra(isPreTool bool, msg string) {
	if isPreTool {
		emitDeny("PPG_GUARD_ERROR: " + msg +
			" — denying (fail-closed): the guard cannot verify this edit. " +
			"Fix the gateway/state setup, or re-lock your plan, and retry.")
		os.Exit(0)
	}
	fmt.Fprintln(os.Stderr, "ppg-copilot-guard: "+msg)
	emitAllow()
	os.Exit(0)
}

// projectDirFallback returns the payload's cwd if present, otherwise the
// process cwd. Only used when neither flag nor env resolves the project dir.
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
// session that locked it, not only with its 15-minute TTL.
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
// omits it. Returns "" when no ticket is available.
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

// decide is the decision function shared in spirit with the Claude guard: it
// gates on the tool name, checks path scope and session binding locally, then
// verifies the edited content against the artifact-view policy through verify.
// A nil verifier skips the content step (used by offline tests).
func decide(payload []byte, rawTicket string, verify artifactVerifier) (bool, string) {
	var in hookInput
	if err := json.Unmarshal(payload, &in); err != nil {
		return true, "PPG_GUARD_ERROR: unreadable hook payload; denying (fail-closed)."
	}
	if !isWriteTool(in.ToolName) {
		return false, "" // read/search/etc. tools are not gated by this guard
	}
	target := in.targetPath()
	if target == "" {
		return true, "PPG_GUARD_ERROR: " + in.ToolName +
			" is a file-mutating tool but no target path was found in tool_input; denying (fail-closed)."
	}
	if smarttools.IsHarnessMetadata(target) {
		return false, "" // harness plan-file bookkeeping, never in ticket scope
	}

	if rawTicket == "" {
		return true, "No capability ticket for this session. " +
			"Lock a plan first: call the lock_in_plan tool (or POST /lock_in_plan on the " +
			"Platform Planning Gateway) — the returned execution_ticket is persisted for you."
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

	if verify != nil {
		if content := in.editedContent(); content != "" {
			violations, err := verify(rawTicket, rel, content)
			if err != nil {
				return true, "PPG_GUARD_ERROR: cannot verify content against policy: " + err.Error() +
					" — denying (fail-closed). Nothing was modified."
			}
			if len(violations) > 0 {
				return true, "ARCHITECTURAL_INVARIANT_VIOLATION: " + strings.Join(violations, " | ") +
					" Nothing was modified; fix the content to satisfy the invariant and resubmit."
			}
		}
	}
	return false, ""
}

// relativeTarget converts the absolute file path Copilot passes into the
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

// gatewayURL is the Platform Planning Gateway base URL (PPG_URL, default
// http://localhost:8765).
func gatewayURL() string {
	if u := os.Getenv("PPG_URL"); u != "" {
		return u
	}
	return "http://localhost:8765"
}

var httpClient = &http.Client{Timeout: 5 * time.Second}

// verifyArtifactRemote asks the gateway to evaluate the artifact-view policy
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
		return nil, fmt.Errorf("decoding gateway response: %w", err)
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
		return nil, fmt.Errorf("unexpected gateway status %q (HTTP %d)", out.Status, resp.StatusCode)
	}
}

func emitDeny(reason string) {
	out := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       "deny",
			"permissionDecisionReason": reason,
		},
	}
	_ = json.NewEncoder(os.Stdout).Encode(out)
}

func emitAllow() {
	// `continue: true` is the neutral "no decision, proceed with normal
	// approval flow" response documented for VS Code Copilot hooks.
	_, _ = os.Stdout.WriteString(`{"continue":true}` + "\n")
}
