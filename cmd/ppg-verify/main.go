// Command ppg-verify is the apply-time / CI backstop of the platform. It
// gathers the working-tree changeset, loads the active capability ticket from
// the per-machine store, and asks the gateway to evaluate the changeset-view
// policy (POST /verify_changeset) over the ACTUAL diff — the enforcement leg
// that covers surfaces with no in-loop hook (the gh copilot CLI, Cursor, a
// human at the terminal, CI).
//
// Wire it as a pre-commit / pre-push hook or a CI step:
//
//	ppg-verify            # verify the working-tree changes vs HEAD
//	ppg-verify --staged   # verify only the staged changes
//	ppg-verify --plan plan.json   # also confirm the plan hash matches the ticket
//
// Exit codes: 0 = changeset accepted; 1 = rejected (violations printed);
// 2 = could not run the check (no ticket, gateway unreachable) — fail closed.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/owulveryck/poc-agentic-platform/internal/plan"
	"github.com/owulveryck/poc-agentic-platform/internal/store"
	"github.com/owulveryck/poc-agentic-platform/internal/version"
)

func main() {
	staged := flag.Bool("staged", false, "verify only staged changes (default: all working-tree changes vs HEAD)")
	planFile := flag.String("plan", "", "path to the locked plan JSON; when set, its hash is checked against the ticket")
	projectDirFlag := flag.String("project-dir", "", "absolute project directory (overrides "+store.EnvProjectDir+")")
	storeRootFlag := flag.String("store-root", "", "per-machine state root (overrides "+store.EnvStoreRoot+")")
	gateway := flag.String("gateway", gatewayURL(), "Platform Planning Gateway base URL")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("ppg-verify " + version.String())
		return
	}

	if err := run(*staged, *planFile, *projectDirFlag, *storeRootFlag, *gateway); err != nil {
		fmt.Fprintln(os.Stderr, "ppg-verify: "+err.Error())
		os.Exit(2) // could not run the check: fail closed
	}
}

func run(staged bool, planFile, projectDirFlag, storeRootFlag, gateway string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, err := store.ResolveRoot(storeRootFlag)
	if err != nil {
		return err
	}
	projectDir, err := store.ResolveProjectDir(projectDirFlag, wd)
	if err != nil {
		return err
	}
	st, err := store.NewFilesystem(root, projectDir)
	if err != nil {
		return fmt.Errorf("cannot open store: %w", err)
	}
	ticket, err := activeTicket(st)
	if err != nil {
		return err
	}

	files, err := changedFiles(staged)
	if err != nil {
		return fmt.Errorf("computing changeset: %w", err)
	}
	if len(files) == 0 {
		fmt.Println("ppg-verify: no changes to verify.")
		return nil
	}

	planHash := ""
	if planFile != "" {
		planHash, err = hashPlan(planFile)
		if err != nil {
			return err
		}
	}

	status, violations, err := verifyChangeset(gateway, ticket, files, planHash)
	if err != nil {
		return err
	}
	if status == "CHANGESET_OK" {
		fmt.Printf("ppg-verify: OK — %d file(s) verified against the locked plan.\n", len(files))
		return nil
	}
	// Rejected: print the reasons and exit 1 (distinct from the fail-closed 2).
	fmt.Fprintf(os.Stderr, "ppg-verify: %s\n", status)
	for _, v := range violations {
		fmt.Fprintln(os.Stderr, "  - "+v)
	}
	os.Exit(1)
	return nil
}

// activeTicket loads the ticket bound to the store's active session.
func activeTicket(st interface {
	store.TokenStore
	store.SessionStore
}) (string, error) {
	sid, err := st.GetActive()
	if err != nil {
		return "", fmt.Errorf("no active session (start one, or lock a plan first): %w", err)
	}
	tok, err := st.Get(sid)
	if err != nil {
		return "", fmt.Errorf("no capability ticket for the active session; lock a plan first: %w", err)
	}
	return tok, nil
}

type changedFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Op      string `json:"op"`
}

// changedFiles returns the working-tree (or staged) changes as artifacts with
// their current content. Deletions are skipped (no content to verify).
func changedFiles(staged bool) ([]changedFile, error) {
	args := []string{"status", "--porcelain"}
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}
	var files []changedFile
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if len(line) < 4 {
			continue
		}
		x, y := line[0], line[1] // staged, worktree status
		path := strings.TrimSpace(line[3:])
		// Renames appear as "old -> new": verify the new path.
		if idx := strings.Index(path, " -> "); idx >= 0 {
			path = path[idx+4:]
		}
		path = strings.Trim(path, `"`)
		if staged {
			if x == ' ' || x == '?' || x == 'D' {
				continue // not staged, or a staged deletion
			}
		} else {
			if x == 'D' || y == 'D' {
				continue // a deletion in either index
			}
		}
		content, err := os.ReadFile(path)
		if err != nil {
			continue // vanished / unreadable: nothing to inspect
		}
		files = append(files, changedFile{Path: filepath.ToSlash(path), Content: string(content), Op: "write"})
	}
	return files, nil
}

// hashPlan reads a plan JSON file and returns its canonical hash for comparison
// against the ticket's plan_hash claim.
func hashPlan(planFile string) (string, error) {
	raw, err := os.ReadFile(planFile)
	if err != nil {
		return "", fmt.Errorf("reading plan %s: %w", planFile, err)
	}
	var p plan.Plan
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", fmt.Errorf("parsing plan %s: %w", planFile, err)
	}
	return p.Hash(), nil
}

func gatewayURL() string {
	if u := os.Getenv("PPG_URL"); u != "" {
		return u
	}
	return "http://localhost:8765"
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

// verifyChangeset posts the changeset to the gateway and returns its status and
// any violation messages. A transport error is returned (fail closed).
func verifyChangeset(gateway, ticket string, files []changedFile, planHash string) (string, []string, error) {
	body, _ := json.Marshal(map[string]any{"ticket": ticket, "files": files, "plan_hash": planHash})
	resp, err := httpClient.Post(strings.TrimRight(gateway, "/")+"/verify_changeset",
		"application/json", bytes.NewReader(body))
	if err != nil {
		return "", nil, fmt.Errorf("gateway unreachable at %s: %w", gateway, err)
	}
	defer func() { _ = resp.Body.Close() }()

	var out struct {
		Status     string `json:"status"`
		Violations []struct {
			Message string `json:"message"`
		} `json:"violations"`
		Guidance string `json:"guidance"`
		Expected string `json:"expected"`
		Got      string `json:"got"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", nil, fmt.Errorf("decoding gateway response: %w", err)
	}
	var msgs []string
	for _, v := range out.Violations {
		msgs = append(msgs, v.Message)
	}
	if out.Guidance != "" {
		msgs = append(msgs, out.Guidance)
	}
	return out.Status, msgs, nil
}
