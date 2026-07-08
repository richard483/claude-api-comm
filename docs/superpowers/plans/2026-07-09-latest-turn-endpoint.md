# Latest-Turn Endpoint Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `GET /sessions/{id}/turns/latest`, returning the last turn Claude produced plus a `pending` flag indicating whether another turn is in flight.

**Architecture:** A single SQL statement selects the newest terminal turn (`done`/`error`), falling back to the newest in-flight turn when nothing has finished, and computes `pending` in the same snapshot. The value flows unchanged up through `store` → `manager` → `api`. `model.LatestTurn` embeds `model.Turn` so the JSON body stays flat.

**Tech Stack:** Go 1.25, chi v5 (router), pgx v5 / pgxpool (Postgres), stdlib `testing`.

Spec: `docs/superpowers/specs/2026-07-08-latest-turn-endpoint-design.md`

## Global Constraints

- Branch: `feat/latest-turn-endpoint` (already checked out, branched from `main`).
- Module path is `github.com/richard483/claude-api-comm`. All internal imports use it.
- Postgres-backed tests skip unless `TEST_DATABASE_URL` is set. Task 2 shows how to start a throwaway Postgres.
- Run the full suite serially: `go test -p 1 ./...`. Parallel `go test ./...` against a *fresh* database fails with `duplicate key ... pg_namespace_nspname_index` because each package calls `Migrate` concurrently. This is a pre-existing bug, not something this plan introduces or fixes.
- `gofmt -l .` must print nothing, and `go vet ./...` must be clean, before every commit.
- The API layer must never import `pgx`. The `pgx.ErrNoRows` → `store.ErrNoTurns` mapping in Task 2 is what keeps that true.
- `pending` describes the **session**, not the turn in the body. `status: "done"` together with `pending: true` is a valid, expected combination.

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `internal/model/model.go` | Add `LatestTurn` (embeds `Turn`, adds `Pending`) | 1 |
| `internal/model/latest_turn_test.go` | *Create.* JSON flattening contract | 1 |
| `internal/store/store.go` | Add `ErrNoTurns`, `GetLatestTurn`, `latestTurnQuery` | 2 |
| `internal/store/latest_turn_test.go` | *Create.* The four selection cases, against real Postgres | 2 |
| `internal/manager/manager.go` | Add `GetLatestTurn` (session existence check, then store) | 3 |
| `internal/manager/latest_turn_test.go` | *Create.* Unknown session fails before turn lookup | 3 |
| `internal/api/api.go` | `Service` interface + `latestTurn` handler + route | 4 |
| `internal/api/api_test.go` | *Modify.* `fakeSvc` gains `GetLatestTurn` | 4 |
| `internal/api/latest_turn_test.go` | *Create.* Body shape, 400, 404, route precedence | 4 |
| `internal/api/openapi.yaml` | New path + `LatestTurn` schema | 5 |

Task order is forced by compilation: `cmd/server` passes `*manager.Manager` where `api.Service` is expected, so the manager must have the method (Task 3) before the interface requires it (Task 4).

---

### Task 1: `model.LatestTurn`

**Files:**
- Modify: `internal/model/model.go` (append after the `Turn` struct, around line 56)
- Test: `internal/model/latest_turn_test.go` (create)

**Interfaces:**
- Consumes: `model.Turn` (existing).
- Produces: `model.LatestTurn{ Turn; Pending bool }`. Embedding is load-bearing — `encoding/json` promotes an embedded struct's fields to the top level, which is what keeps the response body flat. Tasks 2–4 all return this type.

- [ ] **Step 1: Write the failing test**

Create `internal/model/latest_turn_test.go`:

```go
package model

import (
	"encoding/json"
	"testing"
)

// The embedded Turn must flatten into the top-level object, so the body is
// exactly the existing turn representation plus one field.
func TestLatestTurnMarshalsFlatWithPending(t *testing.T) {
	lt := LatestTurn{
		Turn:    Turn{Prompt: "what is 2+2", Result: "4", Status: TurnDone},
		Pending: true,
	}

	b, err := json.Marshal(lt)
	if err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["prompt"] != "what is 2+2" {
		t.Errorf("prompt = %v, want it promoted to the top level", got["prompt"])
	}
	if got["result"] != "4" {
		t.Errorf("result = %v", got["result"])
	}
	if got["status"] != "done" {
		t.Errorf("status = %v", got["status"])
	}
	if got["pending"] != true {
		t.Errorf("pending = %v, want true", got["pending"])
	}
	if _, nested := got["Turn"]; nested {
		t.Error(`body has a nested "Turn" key; Turn must be embedded, not a named field`)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run TestLatestTurnMarshalsFlatWithPending`
Expected: FAIL — `undefined: LatestTurn` (build failure).

- [ ] **Step 3: Write minimal implementation**

Append to `internal/model/model.go`:

```go
// LatestTurn is a Turn plus a session-level flag. Pending is true when the
// session has any queued or running turn — it describes the session, not the
// turn carried here, so a done turn with Pending true is normal and means
// "here is the last answer, and a newer turn is still running".
type LatestTurn struct {
	Turn
	Pending bool `json:"pending"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/model/ -run TestLatestTurnMarshalsFlatWithPending -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
test -z "$(gofmt -l .)" && go vet ./internal/model/
git add internal/model/model.go internal/model/latest_turn_test.go
git commit -m "feat(model): add LatestTurn with flat JSON and pending flag"
```

---

### Task 2: `store.GetLatestTurn` and `store.ErrNoTurns`

**Files:**
- Modify: `internal/store/store.go` (add imports; append after `GetTurn`, around line 154)
- Test: `internal/store/latest_turn_test.go` (create)

**Interfaces:**
- Consumes: `model.LatestTurn` (Task 1); existing `Store.CreateSession`, `Store.CreateTurn`, `Store.StartTurn`, `Store.FinishTurn`.
- Produces:
  - `var store.ErrNoTurns error` — returned when the session has zero turns.
  - `func (s *Store) GetLatestTurn(ctx context.Context, sessionID uuid.UUID) (model.LatestTurn, error)`

**Database setup** (needed for this task and Tasks 3–4). Start a throwaway Postgres — do **not** point `TEST_DATABASE_URL` at the production database on rein-druid:

```bash
docker run -d --name capc-test-pg \
  -e POSTGRES_PASSWORD=test -e POSTGRES_USER=test -e POSTGRES_DB=capc \
  -p 55433:5432 postgres:18
until docker exec capc-test-pg pg_isready -U test -d capc; do sleep 1; done
export TEST_DATABASE_URL="postgres://test:test@127.0.0.1:55433/capc?sslmode=disable"
```

Tear it down when the plan is finished: `docker rm -f capc-test-pg`.

- [ ] **Step 1: Write the failing test**

Create `internal/store/latest_turn_test.go`:

```go
package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/richard483/claude-api-comm/internal/model"
)

func seedSession(t *testing.T, s *Store) model.Session {
	t.Helper()
	sess, err := s.CreateSession(context.Background(), model.Session{
		ID: uuid.New(), WorkspacePath: t.TempDir(), Backend: "worktree",
		Label: "t", Owner: "t", Status: model.SessionActive,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return sess
}

// seedTurn inserts a turn and drives it to the requested status.
// It sleeps 1ms so successive turns get strictly increasing created_at values,
// which the ORDER BY in GetLatestTurn relies on.
func seedTurn(t *testing.T, s *Store, sessionID uuid.UUID, prompt string, status model.TurnStatus, result, errText string) model.Turn {
	t.Helper()
	ctx := context.Background()
	turn, err := s.CreateTurn(ctx, model.Turn{
		ID: uuid.New(), SessionID: sessionID, Prompt: prompt, Status: model.TurnQueued,
	})
	if err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	switch status {
	case model.TurnRunning:
		if err := s.StartTurn(ctx, turn.ID); err != nil {
			t.Fatalf("StartTurn: %v", err)
		}
	case model.TurnDone, model.TurnError:
		turn.Status = status
		turn.Result = result
		turn.Error = errText
		if err := s.FinishTurn(ctx, turn); err != nil {
			t.Fatalf("FinishTurn: %v", err)
		}
	}
	turn.Status = status
	time.Sleep(time.Millisecond)
	return turn
}

func TestGetLatestTurnNoTurnsReturnsErrNoTurns(t *testing.T) {
	s := newTestStore(t)
	sess := seedSession(t, s)

	_, err := s.GetLatestTurn(context.Background(), sess.ID)
	if !errors.Is(err, ErrNoTurns) {
		t.Fatalf("err = %v, want ErrNoTurns", err)
	}
}

func TestGetLatestTurnFallsBackToRunningTurn(t *testing.T) {
	s := newTestStore(t)
	sess := seedSession(t, s)
	running := seedTurn(t, s, sess.ID, "what is 2+2", model.TurnRunning, "", "")

	got, err := s.GetLatestTurn(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("GetLatestTurn: %v", err)
	}
	if got.ID != running.ID {
		t.Errorf("id = %v, want the running turn %v", got.ID, running.ID)
	}
	if got.Status != model.TurnRunning || got.Result != "" {
		t.Errorf("got status=%q result=%q, want running with empty result", got.Status, got.Result)
	}
	if !got.Pending {
		t.Error("pending = false, want true while a turn is running")
	}
}

func TestGetLatestTurnPrefersFinishedTurnOverRunningOne(t *testing.T) {
	s := newTestStore(t)
	sess := seedSession(t, s)
	done := seedTurn(t, s, sess.ID, "hello", model.TurnDone, "hi there", "")
	seedTurn(t, s, sess.ID, "what is 2+2", model.TurnRunning, "", "")

	got, err := s.GetLatestTurn(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("GetLatestTurn: %v", err)
	}
	if got.ID != done.ID {
		t.Errorf("id = %v, want the finished turn %v", got.ID, done.ID)
	}
	if got.Result != "hi there" {
		t.Errorf("result = %q, want the last answer", got.Result)
	}
	if !got.Pending {
		t.Error("pending = false, want true because a newer turn is running")
	}
}

func TestGetLatestTurnReturnsErrorTurnRatherThanStaleSuccess(t *testing.T) {
	s := newTestStore(t)
	sess := seedSession(t, s)
	seedTurn(t, s, sess.ID, "hello", model.TurnDone, "hi there", "")
	failed := seedTurn(t, s, sess.ID, "what is 2+2", model.TurnError, "", "workspace unavailable: /home/x")

	got, err := s.GetLatestTurn(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("GetLatestTurn: %v", err)
	}
	if got.ID != failed.ID {
		t.Errorf("id = %v, want the failed turn %v (failures must not be hidden)", got.ID, failed.ID)
	}
	if got.Status != model.TurnError || got.Error != "workspace unavailable: /home/x" {
		t.Errorf("got status=%q error=%q", got.Status, got.Error)
	}
	if got.Pending {
		t.Error("pending = true, want false when nothing is in flight")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/store/ -run TestGetLatestTurn`
Expected: FAIL — `s.GetLatestTurn undefined` and `undefined: ErrNoTurns` (build failure).

If instead every test reports `SKIP`, `TEST_DATABASE_URL` is unset — fix that before continuing, or you are testing nothing.

- [ ] **Step 3: Write minimal implementation**

In `internal/store/store.go`, change the import block to:

```go
import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/richard483/claude-api-comm/internal/model"
)
```

Then append after `GetTurn`:

```go
// ErrNoTurns means the session exists but has never been given a message.
// Mapped from pgx.ErrNoRows here so callers above this layer never import pgx.
var ErrNoTurns = errors.New("no turns for session")

// latestTurnQuery picks the newest terminal turn (done or error), falling back
// to the newest in-flight turn when none has finished — that is exactly what
// "ORDER BY terminal DESC, created_at DESC" expresses. pending is computed in
// the same statement so the turn and the flag come from one consistent
// snapshot; two separate queries could observe the turn finishing in between
// and report a stale turn with pending=false.
const latestTurnQuery = `
WITH ranked AS (
	SELECT id, session_id, prompt, status, result, error, num_turns, cost_usd,
	       duration_ms, created_at, finished_at,
	       (status IN ('done','error')) AS terminal
	FROM comm_agent_memory.turns
	WHERE session_id = $1
)
SELECT r.id, r.session_id, r.prompt, r.status, r.result, r.error,
       r.num_turns, r.cost_usd, r.duration_ms, r.created_at, r.finished_at,
       EXISTS (SELECT 1 FROM ranked WHERE NOT terminal) AS pending
FROM ranked r
ORDER BY r.terminal DESC, r.created_at DESC
LIMIT 1`

func (s *Store) GetLatestTurn(ctx context.Context, sessionID uuid.UUID) (model.LatestTurn, error) {
	var m model.LatestTurn
	err := s.pool.QueryRow(ctx, latestTurnQuery, sessionID).
		Scan(&m.ID, &m.SessionID, &m.Prompt, &m.Status, &m.Result, &m.Error,
			&m.NumTurns, &m.CostUSD, &m.DurationMs, &m.CreatedAt, &m.FinishedAt, &m.Pending)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.LatestTurn{}, ErrNoTurns
	}
	return m, err
}
```

The `Scan` targets `m.ID`, `m.Prompt`, … directly: those are the embedded `Turn`'s promoted fields.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/store/ -run TestGetLatestTurn -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
test -z "$(gofmt -l .)" && go vet ./internal/store/
git add internal/store/store.go internal/store/latest_turn_test.go
git commit -m "feat(store): add GetLatestTurn selecting last terminal turn with pending flag"
```

---

### Task 3: `manager.GetLatestTurn`

**Files:**
- Modify: `internal/manager/manager.go` (append next to the other pass-through getters, around line 74)
- Test: `internal/manager/latest_turn_test.go` (create)

**Interfaces:**
- Consumes: `store.GetLatestTurn`, `store.ErrNoTurns` (Task 2); existing `Store.GetSession`.
- Produces: `func (m *Manager) GetLatestTurn(ctx context.Context, sessionID uuid.UUID) (model.LatestTurn, error)` — this is the method that lets `*Manager` satisfy the widened `api.Service` in Task 4.

The session lookup happens first so an unknown session id is distinguishable from a session that simply has no turns. Without it, both would surface as `ErrNoTurns`.

- [ ] **Step 1: Write the failing test**

Create `internal/manager/latest_turn_test.go`. It reuses `newMgr` from `manager_test.go` (same package), which skips when `TEST_DATABASE_URL` is unset:

```go
package manager

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/richard483/claude-api-comm/internal/store"
)

// An unknown session must fail on the session lookup, not be reported as a
// session that merely has no turns yet.
func TestGetLatestTurnUnknownSessionIsNotErrNoTurns(t *testing.T) {
	m, _ := newMgr(t)

	_, err := m.GetLatestTurn(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("GetLatestTurn on an unknown session returned nil error")
	}
	if errors.Is(err, store.ErrNoTurns) {
		t.Errorf("unknown session reported as ErrNoTurns: %v", err)
	}
}

func TestGetLatestTurnKnownSessionWithNoTurns(t *testing.T) {
	m, _ := newMgr(t)
	ctx := context.Background()

	sess, err := m.CreateSession(ctx, "demo", "rein")
	if err != nil {
		t.Fatal(err)
	}

	_, err = m.GetLatestTurn(ctx, sess.ID)
	if !errors.Is(err, store.ErrNoTurns) {
		t.Fatalf("err = %v, want ErrNoTurns", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/manager/ -run TestGetLatestTurn`
Expected: FAIL — `m.GetLatestTurn undefined` (build failure).

- [ ] **Step 3: Write minimal implementation**

In `internal/manager/manager.go`, add after the existing `GetTurn` pass-through:

```go
// GetLatestTurn resolves the session first so that an unknown session id is
// reported as such, rather than as a session with no turns.
func (m *Manager) GetLatestTurn(ctx context.Context, sessionID uuid.UUID) (model.LatestTurn, error) {
	if _, err := m.st.GetSession(ctx, sessionID); err != nil {
		return model.LatestTurn{}, err
	}
	return m.st.GetLatestTurn(ctx, sessionID)
}
```

No new imports: `context`, `uuid`, and `model` are already imported by this file.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/manager/ -run TestGetLatestTurn -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
test -z "$(gofmt -l .)" && go vet ./internal/manager/
git add internal/manager/manager.go internal/manager/latest_turn_test.go
git commit -m "feat(manager): add GetLatestTurn with session existence check"
```

---

### Task 4: API route, handler, and `Service` method

**Files:**
- Modify: `internal/api/api.go` (`Service` interface ~line 15; route table ~line 53; new handler after `getTurn` ~line 141)
- Modify: `internal/api/api_test.go` (add a method to `fakeSvc`, ~line 44)
- Test: `internal/api/latest_turn_test.go` (create)

**Interfaces:**
- Consumes: `model.LatestTurn` (Task 1); `*manager.Manager.GetLatestTurn` (Task 3) — this is what makes `cmd/server` still compile once the interface grows.
- Produces: route `GET /sessions/{id}/turns/latest`; `Service.GetLatestTurn(ctx, sessionID) (model.LatestTurn, error)`.

chi matches a static path segment ahead of a param segment regardless of registration order, so `/turns/latest` is not swallowed by the existing `/turns/{turnID}` route. Step 1 pins that with a test rather than trusting it.

`fakeSvc` must gain the new method in the *same* commit as the interface change, otherwise the `api` package's own tests will not build.

- [ ] **Step 1: Write the failing tests**

First, add the fake's fields and method. In `internal/api/api_test.go`, replace the `fakeSvc` struct declaration (lines 15–18) with:

```go
type fakeSvc struct {
	sess      model.Session
	turn      model.Turn
	latest    model.LatestTurn
	latestErr error
}
```

and add this method right after the existing `GetTurn` method (line 30):

```go
func (f *fakeSvc) GetLatestTurn(context.Context, uuid.UUID) (model.LatestTurn, error) {
	return f.latest, f.latestErr
}
```

Then create `internal/api/latest_turn_test.go`:

```go
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/richard483/claude-api-comm/internal/model"
)

func TestLatestTurnReturnsFlatBodyWithPending(t *testing.T) {
	svc := &fakeSvc{latest: model.LatestTurn{
		Turn:    model.Turn{ID: uuid.New(), Prompt: "hello", Result: "hi there", Status: model.TurnDone},
		Pending: true,
	}}
	r := NewRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/sessions/"+uuid.New().String()+"/turns/latest", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["prompt"] != "hello" || got["result"] != "hi there" {
		t.Errorf("prompt/result not at top level: %v", got)
	}
	if got["status"] != "done" {
		t.Errorf("status = %v", got["status"])
	}
	if got["pending"] != true {
		t.Errorf("pending = %v, want true", got["pending"])
	}
}

func TestLatestTurnRejectsInvalidSessionID(t *testing.T) {
	r := NewRouter(&fakeSvc{})

	req := httptest.NewRequest(http.MethodGet, "/sessions/not-a-uuid/turns/latest", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body)
	}
	// Assert on the message, not just the code. Before the route exists this
	// path falls through to getTurn, which also answers 400 — but with
	// "invalid turn id". Without this check the test would pass vacuously.
	var got map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["error"] != "invalid id" {
		t.Errorf("error = %q, want %q (request reached getTurn, not latestTurn)", got["error"], "invalid id")
	}
}

func TestLatestTurnReturns404WhenSessionHasNoTurns(t *testing.T) {
	svc := &fakeSvc{latestErr: errors.New("no turns for session")}
	r := NewRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/sessions/"+uuid.New().String()+"/turns/latest", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body)
	}
}

// "latest" must not be parsed as a turn id by the /turns/{turnID} route.
func TestLatestTurnRouteIsNotSwallowedByGetTurn(t *testing.T) {
	svc := &fakeSvc{
		turn:   model.Turn{Prompt: "WRONG-came-from-getTurn"},
		latest: model.LatestTurn{Turn: model.Turn{Prompt: "RIGHT-came-from-latestTurn"}},
	}
	r := NewRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/sessions/"+uuid.New().String()+"/turns/latest", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body)
	}
	var got model.LatestTurn
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Prompt != "RIGHT-came-from-latestTurn" {
		t.Errorf("prompt = %q; the request reached getTurn instead of latestTurn", got.Prompt)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/ -run TestLatestTurn -v`

It builds — `fakeSvc` may carry an extra method the `Service` interface does not require yet. All four tests must nonetheless FAIL, each for a *specific* reason. With no `/turns/latest` route, chi matches `/turns/{turnID}` with `turnID = "latest"`, so every request lands in `getTurn`:

| Test | Expected failure |
|---|---|
| `TestLatestTurnReturnsFlatBodyWithPending` | `status = 400, want 200` (getTurn rejects `"latest"` as a uuid) |
| `TestLatestTurnRejectsInvalidSessionID` | `error = "invalid turn id", want "invalid id"` — the status is already 400, so only the message proves the route is missing |
| `TestLatestTurnReturns404WhenSessionHasNoTurns` | `status = 400, want 404` |
| `TestLatestTurnRouteIsNotSwallowedByGetTurn` | `status = 400, want 200` |

If any of them passes at this point, stop: it is not testing what you think it is.

- [ ] **Step 3: Write minimal implementation**

In `internal/api/api.go`, add to the `Service` interface, directly after the `GetTurn` line:

```go
	GetLatestTurn(ctx context.Context, sessionID uuid.UUID) (model.LatestTurn, error)
```

Add the route, directly after the existing `r.Get("/sessions/{id}/turns/{turnID}", h.getTurn)` line:

```go
	r.Get("/sessions/{id}/turns/latest", h.latestTurn)
```

And add the handler after `getTurn`:

```go
func (h *handler) latestTurn(w http.ResponseWriter, r *http.Request) {
	sid, err := pathUUID(r, "id")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	lt, err := h.svc.GetLatestTurn(r.Context(), sid)
	if err != nil {
		httpErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, lt)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/ -v -run TestLatestTurn`
Expected: PASS (4 tests).

Then confirm nothing else broke, in particular that `*manager.Manager` still satisfies `Service`:

Run: `go build ./... && go test -p 1 ./...`
Expected: all packages `ok`.

- [ ] **Step 5: Commit**

```bash
test -z "$(gofmt -l .)" && go vet ./...
git add internal/api/api.go internal/api/api_test.go internal/api/latest_turn_test.go
git commit -m "feat(api): add GET /sessions/{id}/turns/latest"
```

---

### Task 5: OpenAPI documentation

**Files:**
- Modify: `internal/api/openapi.yaml` (new path before `/sessions/{id}/turns/{turnID}` at line 97; new schema after `Turn` at ~line 272)

**Interfaces:**
- Consumes: existing `#/components/parameters/SessionId`, `#/components/schemas/Turn`, `#/components/responses/BadRequest`, `#/components/responses/NotFound`.
- Produces: `#/components/schemas/LatestTurn`.

The yaml is served at `/openapi.yaml` and rendered by the `/docs` page, so a malformed file breaks the docs endpoint. `internal/api/docs_test.go` parses it — that is the regression guard.

- [ ] **Step 1: Add the path**

Insert immediately **before** the `/sessions/{id}/turns/{turnID}:` block:

```yaml
  /sessions/{id}/turns/latest:
    get:
      tags: [turns]
      summary: Get the last turn of a session
      description: >
        Returns the newest finished turn (status done or error). If no turn has finished yet,
        returns the in-flight turn with an empty result. `pending` is true when the session has
        any queued or running turn — it describes the session, not the turn in the body, so
        `status: done` together with `pending: true` is normal and means "here is the last
        answer, and a newer turn is still running". A failed turn is returned as-is rather than
        hidden behind an older success. A session that exists but has never been given a
        message returns 404.
      parameters:
        - $ref: "#/components/parameters/SessionId"
      responses:
        "200":
          description: The last turn, plus whether the session has work in flight
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/LatestTurn"
        "400": { $ref: "#/components/responses/BadRequest" }
        "404": { $ref: "#/components/responses/NotFound" }
```

- [ ] **Step 2: Add the schema**

Insert immediately **after** the `Turn:` schema block (before `SummarizeResponse:`):

```yaml
    LatestTurn:
      allOf:
        - $ref: "#/components/schemas/Turn"
        - type: object
          properties:
            pending:
              type: boolean
              description: True when the session has a queued or running turn.
```

- [ ] **Step 3: Verify the yaml still parses and the docs endpoint works**

Run: `go test ./internal/api/ -v`
Expected: PASS, including the existing docs test.

Run a quick structural check that both anchors landed:

```bash
grep -n 'turns/latest' internal/api/openapi.yaml
grep -n 'LatestTurn:' internal/api/openapi.yaml
```
Expected: one hit each.

- [ ] **Step 4: Run the whole suite once more**

Run: `test -z "$(gofmt -l .)" && go vet ./... && go test -p 1 ./...`
Expected: no gofmt output, no vet output, all packages `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/api/openapi.yaml
git commit -m "docs(api): document GET /sessions/{id}/turns/latest in openapi"
```

---

## Manual verification

After Task 5, exercise the real endpoint rather than trusting tests alone. With `TEST_DATABASE_URL` still exported:

```bash
DATABASE_URL="$TEST_DATABASE_URL" LISTEN_ADDR=:18111 \
  WORKSPACE_BASE_DIR=/tmp/capc-ws CLAUDE_BIN=/bin/true \
  go run ./cmd/server &

SID=$(curl -sX POST localhost:18111/sessions -d '{"label":"t","owner":"t"}' | jq -r .id)

# session exists, zero turns -> 404 "no turns for session"
curl -s -o /dev/null -w '%{http_code}\n' localhost:18111/sessions/$SID/turns/latest

# unparseable id -> 400
curl -s -o /dev/null -w '%{http_code}\n' localhost:18111/sessions/not-a-uuid/turns/latest
```

Expected: `404`, then `400`. Kill the server afterwards (`kill %1`) and remove the test database (`docker rm -f capc-test-pg`).

`CLAUDE_BIN=/bin/true` means a posted message finishes immediately with no result event, which is enough to see a terminal turn appear; it is not a substitute for the unit tests.

## Done when

- [ ] `GET /sessions/{id}/turns/latest` returns the last terminal turn, or the in-flight turn when none has finished.
- [ ] `pending` is true exactly when the session has a queued or running turn.
- [ ] An `error` turn is returned as-is, never hidden behind an older success.
- [ ] Zero turns → 404; unparseable id → 400.
- [ ] `test -z "$(gofmt -l .)"` passes, `go vet ./...` clean, `go test -p 1 ./...` green.
- [ ] `openapi.yaml` documents the path and `LatestTurn` schema.
