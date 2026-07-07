// Command ppg-guard is a Claude Code PreToolUse hook: it verifies every
// Edit/Write against the capability ticket locked through the Platform
// Planning Gateway.
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

// hookInput is the subset of the PreToolUse payload the guard needs.
type hookInput struct {
	ToolName  string `json:"tool_name"`
	CWD       string `json:"cwd"`
	ToolInput struct {
		FilePath string `json:"file_path"`
	} `json:"tool_input"`
}

func main() {
	payload, err := os.ReadFile("/dev/stdin")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ppg-guard: cannot read hook payload: %v\n", err)
		os.Exit(1) // non-blocking: broken harness must not lock the session
	}
	block, msg := decide(payload, readTicket(payload))
	if block {
		fmt.Fprintln(os.Stderr, msg)
		os.Exit(2) // blocking: stderr goes back to the model
	}
	os.Exit(0) // no decision: normal permission flow applies
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
	if _, err := smarttools.GuardTargets(rawTicket, []string{target}); err != nil {
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
