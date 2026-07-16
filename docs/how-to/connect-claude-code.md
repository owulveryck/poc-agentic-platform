# How to connect Claude Code (MCP + hooks)

> Full setup in [adapters/claudecode/README.md](../../adapters/claudecode/README.md);
> full worked session in the
> [Claude Code tutorial](../tutorials/02-claude-code-end-to-end.md).
> The short version:

1. **Install the binaries** (once, from the repo checkout):

   ```bash
   make install
   ```

2. **Planning (pillar 1)**: register the MCP server:

   ```bash
   claude mcp add ppg -- ppg-mcp-server
   ```

   Claude Code now sees `get_platform_guidelines_for_intent` and
   `lock_in_plan` as native tools. A successful lock persists the
   capability ticket through the per-machine TokenStore at
   `$XDG_STATE_HOME/ppg/projects/<slug>/tickets/<session_id>` (see
   [capability-ticket.md](../reference/capability-ticket.md#storage-layout)).

3. **Gating (pillar 2)**: register `ppg-guard` (installed in step 1) as a
   `PreToolUse` hook on `Edit|Write` in `.claude/settings.json`:

   ```json
   { "hooks": { "PreToolUse": [ { "matcher": "Edit|Write",
       "hooks": [ { "type": "command", "command": "ppg-guard", "args": [] } ] } ] } }
   ```

4. **Contract**: copy [adapters/claudecode/CLAUDE.example.md](../../adapters/claudecode/CLAUDE.example.md)
   into the target project's `CLAUDE.md`.

5. **Verify**: ask Claude to edit a file outside the locked plan: the hook
   exits with code 2 *before* the edit executes, and the model receives the
   semantic refusal (`OUT_OF_PLAN_SCOPE: ... re-plan through lock_in_plan`),
   which steers it back to the paved road on its own.
