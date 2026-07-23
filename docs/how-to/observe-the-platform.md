# Observe the platform

Every PPG component journals its governance decisions as wide events — one
self-contained JSONL record per decision — into a single per-machine file:

```
<state-root>/events.jsonl        # default: ~/.local/state/ppg/events.jsonl
```

The journal is the *observation* pillar: it is what tells you which guardrails
fired, which plans had to be asked twice, and where an agent tried to step
outside its capability — the raw material for improving the corpus.

## Watch it live

The validation server serves an embedded dashboard:

```
http://127.0.0.1:8765/events
```

It tails the journal over Server-Sent Events (`GET /events/stream`, written by
the server itself *and* by the guards, the MCP server, and `ppg-verify`) and
shows, per session: loop **turns** (one turn = one `ppg.plan.locked`), intents,
plan rejections, guard blocks/allows, verify outcomes — plus red flags
(`asked-twice`, `re-locked`, `substitution`, `bypass?`, `unverified-window`).

**Click any event row** to open the full record in a modal: promoted fields,
attributes, and — where captured — the **request** (e.g. the submitted plan)
and the **reply** (the verdict the agent received: violations, guidance, or
the guard's block message). Close with the button, the backdrop, or Escape.

## Watch the loop itself

```
http://127.0.0.1:8765/events/loop
```

The second view draws the governed loop (Capture the intent → Plan → Act →
Observe, the self-correction loop with its (n) counter, the termination exit
to Result) together with the real PPG components — ppg-mcp-server, ppg-guard,
and the ppg platform with its policy corpus and ticket store.

The view shows **one session at a time** (selector, default: the latest
active one). Selecting a session instantly reconstructs where its loop has
already passed, from the last ~1000 journal events:

- **visit badges** on every box (×N visits, plus a danger !M chip when M
  refusals touched it), and travelled paths thicken with each passage;
- **numbered events**: every narration line carries its #N chip (colored by
  severity) and the icon of its zone; hovering a line lights the zone on the
  diagram, hovering a zone highlights its lines in the list; clicking a line
  replays that single event's animation;
- **tooltips**: hovering a zone lists its last events (#N, time, summary);
- the (n) badge shows the current run of plan rejections and the total
  passages through the self-correction loop; **turns** counts one per locked
  plan.

Live events of the displayed session animate as they arrive (dot along the
path, zone lit: accent = activity, success = allowed, danger = denied) with a
narration line explaining each step. **Replay this session** replays the
whole recorded history at reading speed.

The view opens in a **light theme** derived from the design tokens
(`color-mix` — the canonical palette itself is never modified); the button in
the header toggles back to the dark theme (persisted per browser).

`GET /events/stream?replay=N` replays the last N events on connect (default
50, max 1000). After an EventSource reconnect, replayed rows can duplicate —
cosmetic, not deduplicated in the PoC.

The dashboard reaches every color through `var(--color-*)`: the palette is
injected at serve time from `design/tokens.css` (`-design-tokens` flag) so the
page itself honors the design-token invariant. No tokens file → unstyled but
functional page.

## Analyze it after the fact

```
ppg report                     # per-session tables + top violated invariants
ppg report -json | jq .        # the same aggregation as JSON
ppg report -session 3f2a       # one session (prefix match)
ppg report -since 24h          # recent events only
```

Definitions used by both the report and the dashboard:

- **turn** — one `ppg.plan.locked`: a full pass of the governed
  intent→plan→act→observe loop.
- **asked-twice** — ≥2 consecutive `ppg.plan.rejected` for a session with no
  lock in between (`MAX-REJECT-RUN` column). These rejections were invisible
  before the journal: only repeated-conflict escalations were persisted.
- **re-locked** — ≥2 locks in one session (the plan scope grew mid-session).
- **bypass indicators** — any `ppg.plan.substitution`; guard blocks with
  `session_mismatch` or `ticket_rejected`; `ppg.verify.run` with
  `outcome=error` (the apply-time backstop could not run: an unverified
  window).

Ad-hoc exploration works with any JSONL tooling:

```sh
jq -r 'select(.severity=="WARN") | [.time,.name,.session_id[:8]] | @tsv' \
  ~/.local/state/ppg/events.jsonl

duckdb -c "SELECT name, count(*) FROM read_json_auto('$HOME/.local/state/ppg/events.jsonl')
           GROUP BY 1 ORDER BY 2 DESC"
```

## Operate it

- **Kill switch**: `PPG_TELEMETRY=off` (or `0`/`false`) disables emission in
  any component. Emission is best-effort by design: a telemetry failure is
  logged to stderr and never fails, delays, or alters the decision.
- **Payload capture**: decision events carry bounded (32 KiB) `request` /
  `response` / `reply` attributes — the submitted plan, the returned verdict,
  the guard's block message — which is what the dashboard modal displays.
  `PPG_TELEMETRY_PAYLOADS=off` strips them while keeping the events. File
  contents, edit payloads, and the execution ticket are never captured;
  violation *messages* may quote governed content, so turn payloads off
  before exporting the journal beyond the machine.
- **Rotation**: at process start, a journal over 64 MiB is renamed to
  `events-<unix-ts>.jsonl`. `ppg report` reads rotated files too; the live
  stream loses the handful of events written between its last poll and the
  rename.
- **Privacy contract**: events carry paths, hashes, policy ids, counts, and
  the plan intent — never file contents, edit payloads, or violation message
  bodies.
- **Which component emits what** is a fixed contract: see the
  [telemetry event reference](../reference/telemetry-events.md).

## Export to OpenTelemetry (later)

The `journal.Event` fields map 1:1 onto the OTel Logs data model
(`Time`→Timestamp, `Name`→EventName, `Severity`→SeverityText,
`Component`→`service.name`, `SessionID`→`session.id`, `Attrs`→Attributes). An
OTLP exporter is therefore a pure adapter over the existing file — no schema
migration, no change to any emitter. It is intentionally not part of the PoC.
