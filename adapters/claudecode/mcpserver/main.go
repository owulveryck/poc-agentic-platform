// Command mcpserver exposes the Platform Planning Gateway to Claude Code as
// a stdio MCP server (https://modelcontextprotocol.io/).
//
// Two tools bridge to the gateway's HTTP API:
//
//   - get_platform_guidelines_for_intent → POST $PPG_URL/enrich
//   - lock_in_plan                       → POST $PPG_URL/lock_in_plan
//
// On a successful lock, the execution ticket is also written to .ppg-ticket
// in the current project, where the ppg-guard PreToolUse hook picks it up —
// closing the loop between pillar 1 (amplified planning) and pillar 2
// (in-tool gating) inside a stock Claude Code session.
//
// Session binding: if a .ppg-session file exists (written by the ppg-guard
// SessionStart hook), its session id overrides the plan's session_id before
// the lock, so the issued ticket is bound to the real Claude Code session —
// the guard rejects it from any other session.
//
// Register it with: claude mcp add ppg -- go run ./adapters/claudecode/mcpserver
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/owulveryck/poc-agentic-platform/internal/plan"
)

const ticketFile = ".ppg-ticket"

// sessionFile is written by the ppg-guard SessionStart hook with the real
// Claude Code session id.
const sessionFile = ".ppg-session"

func gatewayURL() string {
	if u := os.Getenv("PPG_URL"); u != "" {
		return u
	}
	return "http://localhost:8000"
}

// guidelinesArgs is the input of get_platform_guidelines_for_intent.
type guidelinesArgs struct {
	Intent         string   `json:"intent" jsonschema:"the natural-language intent to enrich"`
	RepositoryName string   `json:"repository_name" jsonschema:"name of the repository being worked on"`
	TechStack      []string `json:"tech_stack" jsonschema:"technologies of the repository, e.g. Go, Kubernetes"`
}

func main() {
	server := mcp.NewServer(&mcp.Implementation{Name: "ppg", Version: "0.1.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name: "get_platform_guidelines_for_intent",
		Description: "Retrieve the architectural invariants (ADRs) and guardrails the platform " +
			"associates with an intent. ALWAYS call this before planning.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args guidelinesArgs) (*mcp.CallToolResult, any, error) {
		body, _ := json.Marshal(map[string]any{
			"intent": args.Intent,
			"repository_context": map[string]any{
				"name":       args.RepositoryName,
				"tech_stack": args.TechStack,
			},
		})
		return forward(ctx, "/enrich", body, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "lock_in_plan",
		Description: "MANDATORY before any modification: submit the structured execution plan. " +
			"Returns semantic violations to fix, or locks the plan and stores the capability " +
			"ticket that the platform tools (and the ppg-guard hook) will verify.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, p plan.Plan) (*mcp.CallToolResult, any, error) {
		if sid := sessionIDFromFile("."); sid != "" {
			p.SessionID = sid
		}
		body, _ := json.Marshal(p)
		return forward(ctx, "/lock_in_plan", body, saveTicket)
	})

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("ppg mcp server failed: %v", err)
	}
}

// forward posts the payload to the gateway and returns the raw JSON response
// as tool output — including 4xx payloads, so the model reads the semantic
// violations and self-corrects instead of receiving an opaque error.
func forward(ctx context.Context, route string, body []byte, onSuccess func([]byte)) (*mcp.CallToolResult, any, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, gatewayURL()+route, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode == http.StatusOK && onSuccess != nil {
		onSuccess(raw)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}},
		IsError: resp.StatusCode >= 500,
	}, nil, nil
}

// sessionIDFromFile returns the session id recorded by the SessionStart
// hook, or "" when no session file exists (the agent-provided session_id is
// then kept as-is).
func sessionIDFromFile(dir string) string {
	raw, err := os.ReadFile(filepath.Join(dir, sessionFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// saveTicket persists the execution ticket where the ppg-guard hook reads it.
func saveTicket(raw []byte) {
	var out struct {
		ExecutionTicket string `json:"execution_ticket"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.ExecutionTicket == "" {
		return
	}
	if err := os.WriteFile(ticketFile, []byte(out.ExecutionTicket+"\n"), 0o600); err != nil {
		log.Printf("ppg: cannot persist ticket: %v", err)
	}
}
