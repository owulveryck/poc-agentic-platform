# How to connect Claude Code (MCP + hooks)

> Full setup in [adapters/claudecode/README.md](../../adapters/claudecode/README.md);
> full worked session in the
> [Claude Code tutorial](../tutorials/02-claude-code-end-to-end.md).
> The short version:

1. **Planning (pillar 1)**: register the MCP server in the target project:

   ```bash
   claude mcp add ppg -- go run /path/to/poc-agentic-platform/adapters/claudecode/mcpserver
   ```

   Claude Code now sees `get_platform_guidelines_for_intent` and
   `lock_in_plan` as native tools. A successful lock writes the capability
   ticket to `.ppg-ticket`.

2. **Gating (pillar 2)**: build the guard and register it as a `PreToolUse`
   hook on `Edit|Write` in `.claude/settings.json`:

   ```bash
   go build -o ~/.local/bin/ppg-guard ./adapters/claudecode/guard
   ```

   ```json
   { "hooks": { "PreToolUse": [ { "matcher": "Edit|Write",
       "hooks": [ { "type": "command", "command": "ppg-guard", "args": [] } ] } ] } }
   ```

3. **Contract**: copy [adapters/claudecode/CLAUDE.example.md](../../adapters/claudecode/CLAUDE.example.md)
   into the target project's `CLAUDE.md`.

4. **Verify**: ask Claude to edit a file outside the locked plan: the hook
   exits with code 2 *before* the edit executes, and the model receives the
   semantic refusal (`OUT_OF_PLAN_SCOPE: ... re-plan through lock_in_plan`),
   which steers it back to the paved road on its own.
