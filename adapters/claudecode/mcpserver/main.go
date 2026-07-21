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
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

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

	skillsCache := &skillRegistrationCache{seen: map[string][32]byte{}}
	skillDirs := discoverSkillDirs(projectDir)

	mcp.AddTool(server, &mcp.Tool{
		Name: "lock_in_plan",
		Description: "MANDATORY before any modification: submit the structured execution plan. " +
			"Returns semantic violations to fix, or locks the plan and stores the capability " +
			"ticket that the platform tools (and the ppg-guard hook) will verify.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, p plan.Plan) (*mcp.CallToolResult, any, error) {
		stampSessionID(&p, st)
		body, _ := json.Marshal(p)
		raw, status, err := lockWithRegistrationRetry(ctx, p.SessionID, skillDirs, skillsCache, body)
		if err != nil {
			return nil, nil, err
		}
		if status == http.StatusOK {
			saveTicket(st, p.SessionID)(raw)
		}
		return wrapResult(raw, status), nil, nil
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
	raw, status, err := forwardOnce(ctx, route, body)
	if err != nil {
		return nil, nil, err
	}
	if status == http.StatusOK && onSuccess != nil {
		onSuccess(raw)
	}
	return wrapResult(raw, status), nil, nil
}

// lockWithRegistrationRetry uploads any local skills for sessionID (via the
// content-hash cache), forwards the plan body to /lock_in_plan, and — if the
// gateway responds with any unknown_skill violations — invalidates those cache
// entries, re-uploads, and forwards exactly once more. Bounded at one retry so
// a permanently-missing skill still surfaces the semantic error to the model
// instead of looping.
//
// The retry is what makes the MCP self-heal a gateway restart: without it,
// the cache would suppress re-uploads and every subsequent lock in this MCP
// session would fail with unknown_skill until the skill's content changed on
// disk.
func lockWithRegistrationRetry(ctx context.Context, sessionID string, skillDirs []string, cache *skillRegistrationCache, body []byte) ([]byte, int, error) {
	if sessionID != "" {
		registerLocalSkills(ctx, sessionID, skillDirs, cache)
	}
	raw, status, err := forwardOnce(ctx, "/lock_in_plan", body)
	if err != nil {
		return nil, 0, err
	}
	if sessionID == "" {
		return raw, status, nil
	}
	unknown := unknownSkillsIn(raw)
	if len(unknown) == 0 {
		return raw, status, nil
	}
	for _, name := range unknown {
		cache.forget(sessionID, name)
	}
	registerLocalSkills(ctx, sessionID, skillDirs, cache)
	log.Printf("ppg-mcp-server: retrying lock_in_plan after re-registering %v", unknown)
	return forwardOnce(ctx, "/lock_in_plan", body)
}

// forwardOnce is the low-level POST used by forward and by any handler that
// needs to inspect the response body (e.g. the lock_in_plan retry path).
// It returns the raw response body, the HTTP status, and any transport error.
func forwardOnce(ctx context.Context, route string, body []byte) ([]byte, int, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, gatewayURL()+route, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}
	return raw, resp.StatusCode, nil
}

// wrapResult renders a gateway response as an MCP tool result.
func wrapResult(raw []byte, status int) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}},
		IsError: status >= 500,
	}
}

// unknownSkillsIn scans a lock_in_plan response body for violations tagged
// unknown_skill and returns the skill names those violations refer to. It
// also matches the plain sentinel with no name (defensive: a future gateway
// version might reword the message). An empty return means no retry is
// needed.
func unknownSkillsIn(raw []byte) []string {
	var resp struct {
		Violations []struct {
			PolicyID string `json:"policy_id"`
			Message  string `json:"message"`
		} `json:"violations"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil
	}
	var names []string
	for _, v := range resp.Violations {
		if v.PolicyID != "unknown_skill" {
			continue
		}
		// The message shape today is: ...skill_id "<name>"...
		// Grab the first quoted token; if we can't parse it out, fall back
		// to the empty string so registerLocalSkills re-uploads everything.
		names = append(names, quotedName(v.Message))
	}
	return names
}

// quotedName returns the first double-quoted substring in msg, or "" if
// none. Used to recover the skill name from the unknown_skill message.
func quotedName(msg string) string {
	start := -1
	for i, r := range msg {
		if r != '"' {
			continue
		}
		if start < 0 {
			start = i + 1
			continue
		}
		return msg[start:i]
	}
	return ""
}

// skillRegistrationCache remembers the sha256 of (SKILL.md ‖ SKILL.rego) for
// every skill we have uploaded to the gateway this MCP process's lifetime.
// A restart re-uploads everything — cheap and self-healing.
type skillRegistrationCache struct {
	mu   sync.Mutex
	seen map[string][32]byte // key: "<sessionID>|<name>"
}

func (c *skillRegistrationCache) shouldSkip(sessionID, name string, digest [32]byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := sessionID + "|" + name
	if prior, ok := c.seen[key]; ok && prior == digest {
		return true
	}
	c.seen[key] = digest
	return false
}

// forget drops the cache entry for (sessionID, name) so the next
// registerLocalSkills call re-uploads it. Used by the lock_in_plan retry
// path when the gateway reports unknown_skill — evidence that the gateway
// was restarted (or purged) mid-session and the cache is stale. An empty
// name (returned by unknownSkillsIn when the message can't be parsed)
// forgets *every* skill for the session, forcing a full re-upload.
func (c *skillRegistrationCache) forget(sessionID, name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if name != "" {
		delete(c.seen, sessionID+"|"+name)
		return
	}
	prefix := sessionID + "|"
	for k := range c.seen {
		if strings.HasPrefix(k, prefix) {
			delete(c.seen, k)
		}
	}
}

// discoverSkillDirs returns the skill package roots scanned before every
// lock, in ascending precedence order — later directories win on a duplicate
// skill name because the gateway's session tier is last-write-wins, so a
// project-local package overrides a user-wide install of the same skill:
//
//  1. ~/.claude/skills         — user-wide installs (apm --target claude
//     into the home directory; the governed-workstation how-to's location)
//  2. <project>/.agents/skills — the cross-agent skill directory
//     (agent-skills.io), shared with Copilot and other agents
//  3. <project>/.claude/skills — project-local Claude Code skills
//
// Missing directories are skipped silently at scan time.
func discoverSkillDirs(projectDir string) []string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".claude", "skills"))
	}
	return append(dirs,
		filepath.Join(projectDir, ".agents", "skills"),
		filepath.Join(projectDir, ".claude", "skills"),
	)
}

// registerLocalSkills scans every skill root (see discoverSkillDirs) and
// POSTs /register_skill for each skill package found. Order matters: the
// gateway's session tier is last-write-wins, so the last root in the slice
// has the highest precedence.
func registerLocalSkills(ctx context.Context, sessionID string, skillDirs []string, cache *skillRegistrationCache) {
	for _, dir := range skillDirs {
		scanSkillDir(ctx, sessionID, dir, cache)
	}
}

// scanSkillDir walks one skill root at depth 1 and POSTs /register_skill for
// every `<name>/` that contains a SKILL.md — with or without a SKILL.rego.
// Bad rego surfaces as a 422 from the gateway; we log and continue so a
// broken skill doesn't take down the entire lock_in_plan flow (the gateway
// still fail-closes at evaluation time because the skill is not registered).
func scanSkillDir(ctx context.Context, sessionID, skillsDir string, cache *skillRegistrationCache) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("ppg-mcp-server: cannot read %s: %v", skillsDir, err)
		}
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		mdPath := filepath.Join(skillsDir, e.Name(), "SKILL.md")
		md, err := os.ReadFile(mdPath)
		if err != nil {
			continue // not a skill package
		}
		regoPath := filepath.Join(skillsDir, e.Name(), "SKILL.rego")
		rego, _ := os.ReadFile(regoPath) // absent → tier-0 skill
		name := skillNameFromMD(md, e.Name())
		digest := sha256.Sum256(append(md, rego...))
		if cache.shouldSkip(sessionID, name, digest) {
			continue
		}
		body, _ := json.Marshal(map[string]any{
			"session_id": sessionID,
			"name":       name,
			"skill_md":   string(md),
			"skill_rego": string(rego),
		})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, gatewayURL()+"/register_skill", bytes.NewReader(body))
		if err != nil {
			log.Printf("ppg-mcp-server: register_skill %s: %v", name, err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("ppg-mcp-server: register_skill %s: %v", name, err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			raw, _ := io.ReadAll(resp.Body)
			log.Printf("ppg-mcp-server: register_skill %s: %d %s", name, resp.StatusCode, string(raw))
		}
		_ = resp.Body.Close()
	}
}

// skillNameFromMD extracts the front-matter name of a SKILL.md, falling back
// to the directory name. Same shape as the linter's parser.
func skillNameFromMD(raw []byte, fallback string) string {
	for _, line := range bytes.Split(raw, []byte("\n")) {
		if after, ok := bytes.CutPrefix(line, []byte("name:")); ok {
			name := string(bytes.TrimSpace(after))
			if name != "" {
				return name
			}
		}
	}
	return fallback
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
