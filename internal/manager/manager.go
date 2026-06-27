package manager

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/richard483/claude-api-comm/internal/broker"
	"github.com/richard483/claude-api-comm/internal/executor"
	"github.com/richard483/claude-api-comm/internal/model"
	"github.com/richard483/claude-api-comm/internal/runner"
	"github.com/richard483/claude-api-comm/internal/store"
)

type Manager struct {
	st          *store.Store
	run         runner.SessionRunner
	ex          executor.Executor
	br          *broker.Broker
	sem         chan struct{}
	turnTimeout time.Duration

	mu           sync.Mutex
	workers      map[uuid.UUID]chan uuid.UUID     // sessionID -> queued turn IDs
	sessionLocks map[uuid.UUID]*sync.Mutex        // per-session execution lock (Fix 1)
	running      map[uuid.UUID]context.CancelFunc // sessionID -> cancel for running turn (Fix 5)
}

func New(st *store.Store, run runner.SessionRunner, ex executor.Executor, br *broker.Broker, maxConcurrency int, turnTimeout time.Duration) *Manager {
	return &Manager{
		st: st, run: run, ex: ex, br: br,
		sem:          make(chan struct{}, maxConcurrency),
		turnTimeout:  turnTimeout,
		workers:      make(map[uuid.UUID]chan uuid.UUID),
		sessionLocks: make(map[uuid.UUID]*sync.Mutex),
		running:      make(map[uuid.UUID]context.CancelFunc),
	}
}

// sessionLock returns the per-session mutex, creating it lazily under m.mu.
func (m *Manager) sessionLock(id uuid.UUID) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	mu, ok := m.sessionLocks[id]
	if !ok {
		mu = &sync.Mutex{}
		m.sessionLocks[id] = mu
	}
	return mu
}

func (m *Manager) CreateSession(ctx context.Context, label, owner string) (model.Session, error) {
	id := uuid.New()
	ws, err := m.run.Prepare(ctx, id)
	if err != nil {
		return model.Session{}, err
	}
	return m.st.CreateSession(ctx, model.Session{
		ID: id, WorkspacePath: ws, Backend: m.run.Backend(),
		Label: label, Owner: owner, Status: model.SessionActive,
	})
}

func (m *Manager) ListSessions(ctx context.Context, status string) ([]model.Session, error) {
	return m.st.ListSessions(ctx, status)
}
func (m *Manager) GetSession(ctx context.Context, id uuid.UUID) (model.Session, error) {
	return m.st.GetSession(ctx, id)
}
func (m *Manager) GetTurn(ctx context.Context, id uuid.UUID) (model.Turn, error) {
	return m.st.GetTurn(ctx, id)
}
func (m *Manager) Subscribe(turnID uuid.UUID) (<-chan model.Event, func()) {
	return m.br.Subscribe(turnID)
}

func (m *Manager) EnqueueTurn(ctx context.Context, sessionID uuid.UUID, prompt string) (model.Turn, error) {
	if _, err := m.st.GetSession(ctx, sessionID); err != nil {
		return model.Turn{}, err
	}
	turn, err := m.st.CreateTurn(ctx, model.Turn{
		ID: uuid.New(), SessionID: sessionID, Prompt: prompt, Status: model.TurnQueued,
	})
	if err != nil {
		return model.Turn{}, err
	}
	m.dispatch(sessionID) <- turn.ID
	return turn, nil
}

// dispatch returns the session's FIFO channel, starting its worker goroutine once.
func (m *Manager) dispatch(sessionID uuid.UUID) chan uuid.UUID {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch, ok := m.workers[sessionID]
	if !ok {
		ch = make(chan uuid.UUID, 256)
		m.workers[sessionID] = ch
		go func() {
			for turnID := range ch {
				m.runTurn(sessionID, turnID)
			}
		}()
	}
	return ch
}

func (m *Manager) runTurn(sessionID, turnID uuid.UUID) {
	// Lock ordering: sem first, then session lock.
	m.sem <- struct{}{}
	defer func() { <-m.sem }()

	// Fix 5: create a cancellable context and register it so Archive can cancel it.
	ctx, cancel := context.WithTimeout(context.Background(), m.turnTimeout)
	m.mu.Lock()
	m.running[sessionID] = cancel
	m.mu.Unlock()
	defer func() {
		cancel()
		m.mu.Lock()
		delete(m.running, sessionID)
		m.mu.Unlock()
	}()
	defer m.br.Close(turnID)

	sess, err := m.st.GetSession(ctx, sessionID)
	if err != nil {
		_ = m.st.FinishTurn(ctx, model.Turn{ID: turnID, SessionID: sessionID, Status: model.TurnError, Error: err.Error()})
		m.br.Publish(turnID, model.Event{Type: "error", Text: err.Error()})
		return
	}
	turn, err := m.st.GetTurn(ctx, turnID)
	if err != nil {
		_ = m.st.FinishTurn(ctx, model.Turn{ID: turnID, SessionID: sessionID, Status: model.TurnError, Error: err.Error()})
		m.br.Publish(turnID, model.Event{Type: "error", Text: err.Error()})
		return
	}
	_ = m.st.StartTurn(ctx, turnID)
	turn.Status = model.TurnRunning
	m.br.Publish(turnID, model.Event{Type: "status", Text: "running"})

	prompt := turn.Prompt
	resumeID := ""
	if sess.ClaudeSessionID != nil {
		resumeID = *sess.ClaudeSessionID
	} else if sess.RollingSummary != nil && *sess.RollingSummary != "" {
		// first turn of a forked session: seed prior context
		prompt = fmt.Sprintf("Context from a previous session:\n%s\n\n---\n\n%s", *sess.RollingSummary, prompt)
	}

	// Fix 1: hold the per-session lock around Run + claude session ID capture/update.
	sessMu := m.sessionLock(sessionID)
	sessMu.Lock()
	res, runErr := m.ex.Run(ctx, sess.WorkspacePath, prompt, resumeID, func(ev model.Event) {
		m.br.Publish(turnID, ev)
	})
	if runErr == nil && sess.ClaudeSessionID == nil && res.ClaudeSessionID != "" {
		_ = m.st.UpdateSessionClaudeID(ctx, sessionID, res.ClaudeSessionID)
	}
	sessMu.Unlock()

	if runErr != nil {
		turn.Status = model.TurnError
		turn.Error = runErr.Error()
		_ = m.st.FinishTurn(ctx, turn)
		_ = m.st.TouchSession(ctx, sessionID, model.SessionActive)
		m.br.Publish(turnID, model.Event{Type: "error", Text: runErr.Error()})
		return
	}

	turn.Status = model.TurnDone
	turn.Result = res.Text
	turn.NumTurns = res.NumTurns
	turn.CostUSD = res.CostUSD
	turn.DurationMs = res.DurationMs
	_ = m.st.FinishTurn(ctx, turn)
	_ = m.st.TouchSession(ctx, sessionID, model.SessionActive)
	m.br.Publish(turnID, model.Event{Type: "result", Text: res.Text})
}

func (m *Manager) Summarize(ctx context.Context, sessionID uuid.UUID, fork bool) (model.Session, string, error) {
	sess, err := m.st.GetSession(ctx, sessionID)
	if err != nil {
		return model.Session{}, "", err
	}
	resumeID := ""
	if sess.ClaudeSessionID != nil {
		resumeID = *sess.ClaudeSessionID
	}
	const summaryPrompt = "Summarize this session so far as durable context for continuing later: key goals, decisions, current state, and open tasks. Be concise."

	// Fix 1: acquire sem then session lock (same ordering as runTurn).
	m.sem <- struct{}{}
	defer func() { <-m.sem }()

	sessMu := m.sessionLock(sessionID)
	sessMu.Lock()
	res, err := m.ex.Run(ctx, sess.WorkspacePath, summaryPrompt, resumeID, func(model.Event) {})
	sessMu.Unlock()

	if err != nil {
		return model.Session{}, "", err
	}
	if err := m.st.SetRollingSummary(ctx, sessionID, res.Text); err != nil {
		return model.Session{}, "", err
	}
	if !fork {
		updated, _ := m.st.GetSession(ctx, sessionID)
		return updated, res.Text, nil
	}
	newID := uuid.New()
	ws, err := m.run.Prepare(ctx, newID)
	if err != nil {
		return model.Session{}, "", err
	}
	summary := res.Text
	forked, err := m.st.CreateSession(ctx, model.Session{
		ID: newID, WorkspacePath: ws, Backend: m.run.Backend(),
		Label: sess.Label + " (fork)", Owner: sess.Owner,
		Status: model.SessionActive, RollingSummary: &summary,
	})
	return forked, res.Text, err
}

func (m *Manager) Archive(ctx context.Context, sessionID uuid.UUID) error {
	// Fix 5: cancel any running turn before cleaning up the workspace.
	m.mu.Lock()
	if cancel, ok := m.running[sessionID]; ok {
		cancel()
	}
	m.mu.Unlock()

	sess, err := m.st.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if err := m.run.Cleanup(ctx, sess.WorkspacePath); err != nil {
		return err
	}
	return m.st.TouchSession(ctx, sessionID, model.SessionArchived)
}

// Ready checks that the store is reachable (Fix 6).
func (m *Manager) Ready(ctx context.Context) error {
	return m.st.Ping(ctx)
}
