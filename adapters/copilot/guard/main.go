// Command ppg-copilot-guard is a GitHub Copilot pre-tool hook binary that
// mirrors adapters/claudecode/guard for the Copilot hook contract.
//
// Two events are served through the same binary:
//
//   - SessionStart: records the real session id into .ppg-session (where
//     the lock_in_plan MCP tool picks it up) and purges any leftover
//     .ppg-ticket, so a capability never survives the session that
//     locked it.
//   - PreToolUse: verifies every Edit/Write against the capability
//     ticket locked through the Platform Planning Gateway — signature,
//     TTL, scope, and session binding.
//
// The hook receives a JSON payload on stdin. Unlike Claude Code (which
// reads the exit code and stderr), Copilot expects a JSON decision on
// stdout — see https://code.visualstudio.com/docs/agents/reference/hooks-reference.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/owulveryck/poc-agentic-platform/internal/smarttools"
)

const (
	ticketFile  = ".ppg-ticket"
	sessionFile = ".ppg-session"
)

// hookInput is the subset of the Copilot hook payload the guard needs.
// The Copilot desktop app names the file path field `path`; a few tools
// (and the docs' `editFiles` example) use `file_path`. Accept both.
type hookInput struct {
	HookEventName string `json:"hook_event_name"`
	ToolName      string `json:"tool_name"`
	SessionID     string `json:"session_id"`
	CWD           string `json:"cwd"`
	ToolInput     struct {
		Path     string `json:"path"`
		FilePath string `json:"file_path"`
	} `json:"tool_input"`
}

// targetPath returns whichever of the two known file-path fields is set.
func (h hookInput) targetPath() string {
	if h.ToolInput.Path != "" {
		return h.ToolInput.Path
	}
	return h.ToolInput.FilePath
}

// isEditTool reports whether the tool is one whose targets the guard must
// verify against the locked plan. Copilot uses `Edit` and `Write`; the
// VS Code Copilot Chat name is `editFiles`.
func isEditTool(name string) bool {
	switch name {
	case "Edit", "Write", "editFiles":
		return true
	}
	return false
}

func main() {
	payload, err := os.ReadFile("/dev/stdin")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ppg-copilot-guard: cannot read hook payload: %v\n", err)
		os.Exit(1) // non-blocking: broken harness must not lock the session
	}
	var in hookInput
	_ = json.Unmarshal(payload, &in)

	if in.HookEventName == "SessionStart" {
		if err := recordSession(in); err != nil {
			fmt.Fprintf(os.Stderr, "ppg-copilot-guard: cannot record session: %v\n", err)
		}
		emitAllow()
		return
	}

	block, msg := decide(payload, readTicket(payload))
	if block {
		emitDeny(msg)
		return
	}
	emitAllow()
}

// recordSession persists the session id for the MCP server and purges any
// ticket inherited from a previous session: the capability dies with the
// session that locked it, not only with its 15-minute TTL.
func recordSession(in hookInput) error {
	if in.SessionID == "" {
		return nil
	}
	dir := in.CWD
	if dir == "" {
		dir = "."
	}
	_ = os.Remove(filepath.Join(dir, ticketFile))
	return os.WriteFile(filepath.Join(dir, sessionFile), []byte(in.SessionID+"\n"), 0o600)
}

// readTicket loads .ppg-ticket from the hook's cwd. Empty string if absent.
func readTicket(payload []byte) string {
	var in hookInput
	_ = json.Unmarshal(payload, &in)
	dir := in.CWD
	if dir == "" {
		dir = "."
	}
	raw, err := os.ReadFile(filepath.Join(dir, ticketFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// decide is the pure decision function: given the hook payload and the raw
// ticket, it returns whether to block and the semantic message for the model.
func decide(payload []byte, rawTicket string) (bool, string) {
	var in hookInput
	if err := json.Unmarshal(payload, &in); err != nil {
		return false, "" // unparseable payload: stay out of the way
	}
	if !isEditTool(in.ToolName) {
		return false, "" // read/search/etc. tools are not gated by this guard
	}
	target := in.targetPath()
	if target == "" {
		return false, "" // no file path to check
	}

	if rawTicket == "" {
		return true, "No capability ticket found (" + ticketFile + "). " +
			"Lock a plan first: call the lock_in_plan tool (or POST /lock_in_plan on the " +
			"Platform Planning Gateway) and save the execution_ticket to " + ticketFile + "."
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
	return false, ""
}

// relativeTarget converts the absolute file path Copilot passes into the
// project-relative path the plan scope is expressed in.
func relativeTarget(filePath, cwd string) string {
	if cwd == "" || !filepath.IsAbs(filePath) {
		return filepath.ToSlash(filePath)
	}
	rel, err := filepath.Rel(cwd, filePath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(filePath)
	}
	return filepath.ToSlash(rel)
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
