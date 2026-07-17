// Command mcpserver exposes the Platform Planning Gateway to Claude Code as
// a stdio MCP server (https://modelcontextprotocol.io/).
//
// Two tools bridge to the gateway's HTTP API:
//
//   - get_platform_guidelines_for_intent → POST $PPG_URL/enrich
//   - lock_in_plan                       → POST $PPG_URL/lock_in_plan
//
// On a successful lock, the execution ticket is persisted through the
// per-machine TokenStore (default $XDG_STATE_HOME/ppg/projects/<slug>/tickets/<sid>),
// where the ppg-guard PreToolUse hook picks it up — closing the loop
// between pillar 1 (amplified planning) and pillar 2 (in-tool gating)
// inside a stock Claude Code session.
//
// Session binding: if the SessionStore already holds an active session id
// (written by the ppg-guard SessionStart hook), it overrides the plan's
// session_id before the lock, so the issued ticket is bound to the real
// Claude Code session — the guard rejects it from any other session.
//
// The project directory is resolved from --project-dir > PPG_PROJECT_DIR >
// os.Getwd() fallback. In Claude Code and Copilot desktop, MCP servers
// are spawned per-session with cwd = project root, so the fallback is
// reliable for those surfaces; a persistent daemon that outlives a
// project switch should set the env or flag explicitly.
//
// Register it with:
//
//	claude mcp add ppg --scope user \
//	  --env PPG_PROJECT_DIR=/abs/path/to/project \
//	  --env PPG_URL=http://localhost:8765 \
//	  -- "$HOME/.local/bin/ppg-mcp-server"
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/owulveryck/poc-agentic-platform/internal/plan"
	"github.com/owulveryck/poc-agentic-platform/internal/store"
	"github.com/owulveryck/poc-agentic-platform/internal/version"
)

func gatewayURL() string {
	if u := os.Getenv("PPG_URL"); u != "" {
		return u
	}
	return "http://localhost:8765"
}

// guidelinesArgs is the input of get_platform_guidelines_for_intent.
type guidelinesArgs struct {
	Intent         string   `json:"intent" jsonschema:"the natural-language intent to enrich"`
	RepositoryName string   `json:"repository_name" jsonschema:"name of the repository being worked on"`
	TechStack      []string `json:"tech_stack" jsonschema:"technologies of the repository, e.g. Go, Kubernetes"`
}

// discoverArgs is the input of find_platform_service.
type discoverArgs struct {
	Capability     string   `json:"capability" jsonschema:"the capability needed, e.g. notification, payment, storage"`
	Intent         string   `json:"intent" jsonschema:"optional natural-language intent, used when capability is unknown"`
	RepositoryName string   `json:"repository_name" jsonschema:"name of the repository being worked on"`
	TechStack      []string `json:"tech_stack" jsonschema:"technologies of the repository, e.g. Go, Kubernetes"`
}

func main() {
	projectDirFlag := flag.String("project-dir", "",
		"absolute project directory (overrides "+store.EnvProjectDir+" and cwd fallback)")
	storeRootFlag := flag.String("store-root", "",
		"per-machine state root (overrides "+store.EnvStoreRoot+"); defaults to $XDG_STATE_HOME/ppg")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("ppg-mcp-server " + version.String())
		return
	}

	root, err := store.ResolveRoot(*storeRootFlag)
	if err != nil {
		log.Fatalf("ppg-mcp-server: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}
	projectDir, err := store.ResolveProjectDir(*projectDirFlag, cwd)
	if err != nil {
		log.Fatalf("ppg-mcp-server: cannot resolve project dir "+
			"(pass --project-dir, set %s, or spawn the server with a project cwd): %v",
			store.EnvProjectDir, err)
	}
	st, err := store.NewFilesystem(root, projectDir)
	if err != nil {
		log.Fatalf("ppg-mcp-server: cannot open store: %v", err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "ppg", Version: version.String()}, nil)

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
		Name: "find_platform_service",
		Description: "Discover the sanctioned platform service for a capability " +
			"(payment, notification, storage, …). Call this in the plan phase before " +
			"integrating any shared or external capability, and build on the returned " +
			"service, endpoint, and API usage — do not reinvent a client or pick an " +
			"unlisted, deprecated, or forbidden provider; the platform ranks and returns " +
			"the recommended one.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args discoverArgs) (*mcp.CallToolResult, any, error) {
		body, _ := json.Marshal(map[string]any{
			"capability": args.Capability,
			"intent":     args.Intent,
			"repository_context": map[string]any{
				"name":       args.RepositoryName,
				"tech_stack": args.TechStack,
			},
		})
		return forward(ctx, "/discover_service", body, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "lock_in_plan",
		Description: "MANDATORY before any modification: submit the structured execution plan. " +
			"Returns semantic violations to fix, or locks the plan and stores the capability " +
			"ticket that the platform tools (and the ppg-guard hook) will verify.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, p plan.Plan) (*mcp.CallToolResult, any, error) {
		stampSessionID(&p, st)
		body, _ := json.Marshal(p)
		return forward(ctx, "/lock_in_plan", body, saveTicket(st, p.SessionID))
	})

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("ppg mcp server failed: %v", err)
	}
}

// stampSessionID overrides p.SessionID with the SessionStore's active id
// when one is set, so the issued ticket is bound to the real agent session.
// Returns true when a stamp was applied.
func stampSessionID(p *plan.Plan, ss store.SessionStore) bool {
	sid, err := ss.GetActive()
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			log.Printf("ppg-mcp-server: session store: %v", err)
		}
		return false
	}
	if sid == "" {
		return false
	}
	p.SessionID = sid
	return true
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

// saveTicket returns an onSuccess closure that decodes the execution
// ticket out of the lock_in_plan response and persists it through ts,
// keyed by sessionID (the id the ticket was issued for).
func saveTicket(ts store.TokenStore, sessionID string) func([]byte) {
	return func(raw []byte) {
		var out struct {
			ExecutionTicket string `json:"execution_ticket"`
		}
		if err := json.Unmarshal(raw, &out); err != nil || out.ExecutionTicket == "" {
			return
		}
		if sessionID == "" {
			log.Printf("ppg-mcp-server: cannot persist ticket: plan has no session_id")
			return
		}
		if err := ts.Put(sessionID, out.ExecutionTicket); err != nil {
			log.Printf("ppg-mcp-server: cannot persist ticket: %v", err)
		}
	}
}
