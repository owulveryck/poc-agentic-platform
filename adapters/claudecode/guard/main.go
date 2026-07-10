// Command ppg-guard is a Claude Code hook binary serving two events:
//
//   - SessionStart: records the real session id into .ppg-session (where the
//     lock_in_plan MCP tool picks it up) and purges any leftover .ppg-ticket,
//     so a capability never survives the session that locked it.
//   - PreToolUse: verifies every Edit/Write against the capability ticket
//     locked through the Platform Planning Gateway — signature, TTL, scope,
//     and session binding.
//
// Contract (see https://code.claude.com/docs/en/hooks): the hook receives a
// JSON payload on stdin; exit code 2 blocks the tool call and stderr is fed
// back to the model — which turns this hook into the deterministic in-tool
// guard of the amplified loop, running inside an off-the-shelf agent.
//
// The ticket is read from the .ppg-ticket file at the project root (written
// by the lock_in_plan MCP tool, or by hand after a curl to /lock_in_plan).
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

// ticketFile is where the locked plan's ticket lives, relative to the
// project root (the hook's cwd).
const ticketFile = ".ppg-ticket"

// sessionFile is where the current session id lives, written at
// SessionStart and read by the lock_in_plan MCP tool so the issued ticket
// is bound to the real session.
const sessionFile = ".ppg-session"

// hookInput is the subset of the hook payloads the guard needs.
type hookInput struct {
	HookEventName string `json:"hook_event_name"`
	ToolName      string `json:"tool_name"`
	SessionID     string `json:"session_id"`
	CWD           string `json:"cwd"`
	ToolInput     struct {
		FilePath string `json:"file_path"`
	} `json:"tool_input"`
}

func main() {
	payload, err := os.ReadFile("/dev/stdin")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ppg-guard: cannot read hook payload: %v\n", err)
		os.Exit(1) // non-blocking: broken harness must not lock the session
	}
	var in hookInput
	_ = json.Unmarshal(payload, &in)
	if in.HookEventName == "SessionStart" {
		if err := recordSession(in); err != nil {
			fmt.Fprintf(os.Stderr, "ppg-guard: cannot record session: %v\n", err)
		}
		os.Exit(0) // SessionStart never blocks
	}
	block, msg := decide(payload, readTicket(payload))
	if block {
		fmt.Fprintln(os.Stderr, msg)
		os.Exit(2) // blocking: stderr goes back to the model
	}
	os.Exit(0) // no decision: normal permission flow applies
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
	if in.ToolInput.FilePath == "" {
		return false, "" // not a file edition: nothing to guard
	}

	if rawTicket == "" {
		return true, "No capability ticket found (" + ticketFile + "). " +
			"Lock a plan first: call the lock_in_plan tool (or POST /lock_in_plan on the " +
			"Platform Planning Gateway) and save the execution_ticket to " + ticketFile + "."
	}

	target := relativeTarget(in.ToolInput.FilePath, in.CWD)
	claims, err := smarttools.GuardTargets(rawTicket, []string{target})
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

// relativeTarget converts the absolute file path Claude Code passes into the
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
