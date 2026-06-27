# claude-api-comm — deferred follow-ups (v1 known limitations)

These were surfaced during review and consciously deferred. None block v1.

## Lifecycle / scale
- **Idle session reaper**: `IDLE_REAP_AGE` config exists but no reaper runs yet (spec §8).
  Reaping should also drop `sessionLocks`/`running`/worker entries (currently one per session forever).
- **Worker goroutines never reaped**: one goroutine + dispatch channel per distinct session ever used.
- **`sessionLocks` map growth**: never pruned; tie to the reaper.

## Robustness
- **Archive vs in-flight turn**: Archive cancels the running turn (process group killed) then removes the
  workspace immediately; the dying child may survive up to `WaitDelay` (5s). Consider grabbing the session
  lock (or awaiting turn completion) before Cleanup for full synchronization.
- **EnqueueTurn blocking send**: blocks if a session's dispatch buffer (256) fills; select on ctx and
  return 503 instead.
- **Orphaned workspace on partial create**: if the DB insert fails after `Prepare`, the dir leaks
  (also forked sessions in Summarize). Clean up on insert failure.
- **EnqueueTurn doesn't reject archived sessions**: turn fails later with an opaque cwd error.

## Features vs spec
- **ListSessions owner filter**: spec §6 mentions filter by owner; only `status` is wired.
- **migrations/ dir**: spec §7 lists one; DDL is currently embedded idempotent (`IF NOT EXISTS`) in store.go.
  Fine for v1, no versioned migration path.
- **SSE multi-block progress**: `parseLine` emits only the first content block per assistant message,
  so a tool_use following a text block in one message is dropped from the live stream. (Does NOT affect
  the final answer, which comes from the `result` line.)

## Tests
- Broker: add multi-subscriber / cancel-vs-Close ordering / double-cancel cases.
- Executor: add negative-path tests (nonzero exit, missing result line).
- API: getSession/getTurn/SSE/summarize/archive handlers are untested.
- Config: no test for MaxConcurrency<1 validation; no test for unparseable-JSON parse path.

## Deployment (out of code scope)
- systemd unit + Jenkins job mirroring pg-client; optional auth middleware (Karasu) if ever exposed.
- `ContainerRunner` backend for true per-session isolation.
