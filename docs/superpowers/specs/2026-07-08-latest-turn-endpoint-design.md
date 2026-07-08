# Latest-turn endpoint

**Date:** 2026-07-08
**Status:** Approved, not yet implemented

## Problem

A client that posts a message has no cheap way to ask "what did Claude last say, and is
it still working?". Today it must remember the turn id returned by
`POST /sessions/{id}/messages` and poll `GET /sessions/{id}/turns/{turnID}`, or hold an
SSE stream open. A caller that reconnects, restarts, or never kept the turn id is stuck.

## Goal

One endpoint that answers both questions in a single round trip: the last thing Claude
said, plus whether another turn is in flight.

## Endpoint

```
GET /sessions/{id}/turns/latest
```

No request body. Response 200 is the existing turn representation plus one field:

```json
{
  "id": "b88ab0f1-...",
  "session_id": "386addf6-...",
  "prompt": "what is 2+2",
  "status": "done",
  "result": "4",
  "error": "",
  "num_turns": 1,
  "cost_usd": 0.012,
  "duration_ms": 1234,
  "created_at": "2026-07-08T10:26:17Z",
  "finished_at": "2026-07-08T10:26:41Z",
  "pending": false
}
```

`pending` describes the **session**, not the turn in the body. It is true when any turn
of the session is `queued` or `running`. So a body with `status: "done"` and
`pending: true` is normal and means: here is the last answer, and a newer turn is still
being worked on.

## Selection rule

```
if session has 0 turns            -> 404 "no turns for session"
T = newest turn with status in (done, error)      -- by created_at
P = any turn with status in (queued, running)
if T exists  -> return T,            pending = P
else         -> return newest turn,  pending = true
```

Consequences, stated explicitly:

- A failed turn is a real outcome. If the newest terminal turn is an `error`, that is
  what comes back (`status: "error"`, `error` populated, `result: ""`). The endpoint
  never hides a failure behind a stale success.
- If the session's first turn is still running there is no terminal turn, so the running
  turn itself is returned with `result: ""` and `pending: true`.
- A session that exists but has never been given a message is a 404, not an empty body.

## Implementation

Selection happens in one SQL statement, so the returned turn and `pending` come from a
single consistent snapshot. A two-query version could observe the turn finishing between
the two reads and report `pending: false` alongside a stale turn.

```sql
WITH ranked AS (
  SELECT *, (status IN ('done','error')) AS terminal
  FROM comm_agent_memory.turns WHERE session_id = $1
)
SELECT r.id, r.session_id, r.prompt, r.status, r.result, r.error,
       r.num_turns, r.cost_usd, r.duration_ms, r.created_at, r.finished_at,
       EXISTS (SELECT 1 FROM ranked WHERE NOT terminal) AS pending
FROM ranked r
ORDER BY r.terminal DESC, r.created_at DESC
LIMIT 1;
```

`ORDER BY terminal DESC, created_at DESC` is the selection rule: terminal turns first,
newest first within each group, so the fallback to a running turn needs no extra branch.
Zero rows means the session has no turns.

### Layers

| Layer | Change |
|---|---|
| `internal/model` | `LatestTurn` struct: embeds `Turn` (which flattens the JSON), adds a `Pending` bool tagged `pending` |
| `internal/store` | `GetLatestTurn(ctx, sessionID) (model.LatestTurn, error)`; new exported `ErrNoTurns`, mapped from `pgx.ErrNoRows` so the API layer never imports pgx |
| `internal/manager` | `GetLatestTurn(ctx, sessionID)`: `GetSession` first (so an unknown session is distinguishable from an empty one), then `store.GetLatestTurn` |
| `internal/api` | `Service` gains `GetLatestTurn`; handler registered at `/sessions/{id}/turns/latest` |
| `internal/api/openapi.yaml` | New path; `LatestTurn` schema as `allOf: [Turn, {pending}]` |

chi matches a static path segment ahead of a param segment regardless of registration
order (verified), so `/turns/latest` cannot be swallowed by the existing
`/turns/{turnID}` route.

### Status codes

| Case | Code | Body |
|---|---|---|
| Success | 200 | turn fields + `pending` |
| Unparseable session id | 400 | `{"error":"invalid id"}` |
| Session not found | 404 | existing store error text |
| Session exists, zero turns | 404 | `{"error":"no turns for session"}` |

This matches the existing handlers, which map every store error to 404. That means a
database outage on this route also reads as 404. It is a pre-existing wart, kept for
consistency rather than fixed here.

## Testing

Written test-first, each watched failing before implementation.

- **store** (requires `TEST_DATABASE_URL`)
  - zero turns -> `ErrNoTurns`
  - only a running turn -> that turn, `pending: true`
  - done then running -> the **done** turn, `pending: true`
  - done then error -> the **error** turn, `pending: false`
- **api** (fake `Service`)
  - 200 body has flat turn fields plus `pending`
  - unparseable id -> 400
  - `ErrNoTurns` -> 404
  - route precedence: `/turns/latest` reaches the new handler, not `getTurn`
- **manager**
  - unknown session errors before any turn lookup

## Out of scope

- A turn history / list endpoint. Only the last turn was asked for.
- Any new column, index, or migration. `turns_session_idx (session_id, created_at)`
  already covers the scan and turns-per-session is small.
- Splitting 500s out of the blanket-404 error mapping.

## Known limitation

If two turns of one session share an identical `created_at`, their relative order is
undefined. Turns are serialized per session and `now()` advances between inserts, so
this is accepted rather than solved with a monotonic sequence column.
