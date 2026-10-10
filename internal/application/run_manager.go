package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// RunState is the durable lifecycle of one research invocation. None of these
// states represent an order, trade execution, or approval to trade.
type RunState string

const (
	RunPending   RunState = "PENDING"
	RunRunning   RunState = "RUNNING"
	RunCompleted RunState = "COMPLETED"
	RunFailed    RunState = "FAILED"
	RunCancelled RunState = "CANCELLED"
)

const (
	FailureResearch      = "RESEARCH_FAILED"
	FailureInterrupted   = "PROCESS_INTERRUPTED"
	FailureInvalidResult = "INVALID_RESULT"
	FailurePersistence   = "PERSISTENCE_FAILED"
)

// RunRecord stores metadata and the exact typed workflow output JSON, not the
// input portfolio snapshot. Artifact must only exist for COMPLETED runs.
// Caller ownership/authorization must be enforced by the future API layer.
type RunRecord struct {
	ID            string          `json:"run_id"`
	Mode          ResearchMode    `json:"mode"`
	Symbol        string          `json:"symbol"`
	RequestedAsOf time.Time       `json:"requested_as_of"`
	State         RunState        `json:"state"`
	CreatedAt     time.Time       `json:"created_at"`
	StartedAt     *time.Time      `json:"started_at,omitempty"`
	FinishedAt    *time.Time      `json:"finished_at,omitempty"`
	FailureCode   string          `json:"failure_code,omitempty"`
	Artifact      json.RawMessage `json:"artifact,omitempty"`
}

// RunStore is a single-writer durable run repository. Implementations must
// return detached snapshots, rather than exposing shared mutable backing data.
type RunStore interface {
	Save(context.Context, RunRecord) error
	Get(context.Context, string) (RunRecord, error)
	List(context.Context) ([]RunRecord, error)
}

var (
	ErrRunNotFound      = errors.New("research run not found")
	ErrRunNotActive     = errors.New("research run is not active")
	ErrRunCapacity      = errors.New("research queue is at capacity")
	ErrRunManagerClosed = errors.New("research run manager closed")
)

// ResearchExecutor is implemented by the existing ResearchService; a fake can
// replace it in tests. The executor must honor ctx cancellation when possible.
type ResearchExecutor interface {
	Run(context.Context, ResearchRequest) (*ResearchResponse, error)
}

// RunManager schedules bounded, asynchronous calls to the single validated
// ResearchService. It never constructs LLM/Provider dependencies.
// One manager per storage directory is supported; concurrent processes require
// a transactional DB-backed RunStore/claim mechanism instead of FileRunStore.
type RunManager struct {
	executor  ResearchExecutor
	store     RunStore
	mu        sync.Mutex
	active    map[string]context.CancelFunc
	closed    bool
	ctx       context.Context
	cancel    context.CancelFunc
	slots     chan struct{}
	maxActive int
	wg        sync.WaitGroup
	now       func() time.Time
}

// NewRunManager reconciles process-interrupted RUNNING/PENDING runs to FAILED
// before accepting new work, rather than resurrecting unknown paid requests.
// maxParallel limits simultaneous LLM runs; maxActive bounds running + queued.
func NewRunManager(executor ResearchExecutor, store RunStore, maxParallel, maxActive int) (*RunManager, error) {
	if executor == nil || store == nil {
		return nil, errors.New("run manager requires executor and store")
	}
	if maxParallel < 1 || maxActive < maxParallel {
		return nil, errors.New("invalid research concurrency limits")
	}
	existing, err := store.List(context.Background())
	if err != nil {
		return nil, fmt.Errorf("load research runs: %w", err)
	}
	now := time.Now().UTC()
	for _, record := range existing {
		if record.State != RunPending && record.State != RunRunning {
			continue
		}
		record.State = RunFailed
		record.FinishedAt = &now
		record.FailureCode = FailureInterrupted
		record.Artifact = nil
		if err := store.Save(context.Background(), record); err != nil {
			return nil, fmt.Errorf("reconcile interrupted run %s: %w", record.ID, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &RunManager{executor: executor, store: store, active: make(map[string]context.CancelFunc), ctx: ctx, cancel: cancel, slots: make(chan struct{}, maxParallel), maxActive: maxActive, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (m *RunManager) Submit(ctx context.Context, request ResearchRequest) (RunRecord, error) {
	if m == nil {
		return RunRecord{}, errors.New("run manager is nil")
	}
	if ctx == nil {
		return RunRecord{}, errors.New("submit context is nil")
	}
	if err := ctx.Err(); err != nil {
		return RunRecord{}, err
	}
	if err := validateRunRequest(request); err != nil {
		return RunRecord{}, err
	}
	// Freeze caller-owned slices and portfolio pointers before async execution.
	// JSON round-trip also rejects malformed/unserializable financial inputs
	// before storing a task or invoking a paid provider.
	data, err := json.Marshal(request)
	if err != nil {
		return RunRecord{}, fmt.Errorf("snapshot research request: %w", err)
	}
	var frozen ResearchRequest
	if err := json.Unmarshal(data, &frozen); err != nil {
		return RunRecord{}, fmt.Errorf("restore research request snapshot: %w", err)
	}
	request = frozen
	id, err := secureRunID()
	if err != nil {
		return RunRecord{}, err
	}
	now := m.now()
	record := RunRecord{ID: id, Mode: request.Mode, Symbol: strings.TrimSpace(request.Input.Symbol), RequestedAsOf: request.Input.RequestedAsOf, State: RunPending, CreatedAt: now}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return RunRecord{}, ErrRunManagerClosed
	}
	if len(m.active) >= m.maxActive {
		return RunRecord{}, ErrRunCapacity
	}
	if err := ctx.Err(); err != nil {
		return RunRecord{}, err
	}
	// Persist the initial state before starting any potentially paid workflow.
	if err := m.store.Save(ctx, record); err != nil {
		return RunRecord{}, fmt.Errorf("persist new research run: %w", err)
	}
	runCtx, cancel := context.WithCancel(m.ctx)
	m.active[id] = cancel
	m.wg.Add(1)
	go m.execute(runCtx, request, record)
	return record, nil
}

func validateRunRequest(req ResearchRequest) error {
	if req.Mode != ModeDecision && req.Mode != ModePortfolio {
		return fmt.Errorf("invalid research mode %q", req.Mode)
	}
	if strings.TrimSpace(req.Input.Symbol) == "" {
		return errors.New("research symbol is required")
	}
	if req.Input.RequestedAsOf.IsZero() {
		return errors.New("research cutoff is required")
	}
	if req.Mode == ModeDecision && req.Portfolio != nil {
		return errors.New("decision research cannot have portfolio data")
	}
	if req.Mode == ModePortfolio && req.Portfolio == nil {
		return errors.New("portfolio research requires an explicit account snapshot")
	}
	return nil
}

func secureRunID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate run ID: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func (m *RunManager) execute(ctx context.Context, request ResearchRequest, record RunRecord) {
	defer m.wg.Done()
	defer func() { m.mu.Lock(); delete(m.active, record.ID); m.mu.Unlock() }()
	select {
	case m.slots <- struct{}{}:
		defer func() { <-m.slots }()
	case <-ctx.Done():
		m.finish(ctx, record, RunCancelled, "")
		return
	}
	if ctx.Err() != nil {
		m.finish(ctx, record, RunCancelled, "")
		return
	}
	started := m.now()
	record.StartedAt = &started
	record.State = RunRunning
	if err := m.store.Save(context.Background(), record); err != nil {
		// No paid research if the RUNNING transition cannot be recorded.
		m.finish(ctx, record, RunFailed, FailurePersistence)
		return
	}
	response, err := m.executor.Run(ctx, request)
	if ctx.Err() != nil {
		m.finish(ctx, record, RunCancelled, "")
		return
	}
	if err != nil {
		m.finish(ctx, record, RunFailed, FailureResearch)
		return
	}
	if response == nil || response.Mode != request.Mode || response.Output() == nil {
		m.finish(ctx, record, RunFailed, FailureInvalidResult)
		return
	}
	artifact, err := json.Marshal(response.Output())
	if err != nil || len(artifact) == 0 || string(artifact) == "null" {
		m.finish(ctx, record, RunFailed, FailureInvalidResult)
		return
	}
	finished := m.now()
	record.State = RunCompleted
	record.FinishedAt = &finished
	record.Artifact = artifact
	record.FailureCode = ""
	// Failed completion persistence leaves RUNNING on disk; startup recovery
	// converts that into FAILED. Never signal success from an in-memory cache.
	_ = m.store.Save(context.Background(), record)
}

func (m *RunManager) finish(ctx context.Context, record RunRecord, state RunState, code string) {
	finished := m.now()
	record.State = state
	record.FinishedAt = &finished
	record.Artifact = nil
	record.FailureCode = code
	// Even after cancellation, try to persist the terminal state. The store
	// is authoritative; the next startup reconciles any missed transition.
	_ = m.store.Save(context.Background(), record)
}

func (m *RunManager) Get(ctx context.Context, id string) (RunRecord, error) {
	if m == nil {
		return RunRecord{}, errors.New("run manager is nil")
	}
	return m.store.Get(ctx, id)
}
func (m *RunManager) List(ctx context.Context) ([]RunRecord, error) {
	if m == nil {
		return nil, errors.New("run manager is nil")
	}
	return m.store.List(ctx)
}
func (m *RunManager) Cancel(ctx context.Context, id string) error {
	if m == nil {
		return errors.New("run manager is nil")
	}
	if ctx == nil {
		return errors.New("cancel context is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	cancel, ok := m.active[id]
	if ok {
		cancel()
	}
	m.mu.Unlock()
	if !ok {
		return ErrRunNotActive
	}
	return nil
}

// Close cancels active work and persists terminal outcomes before returning.
// Do not invoke it from a workflow runner managed by this RunManager.
func (m *RunManager) Close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if !m.closed {
		m.closed = true
		m.cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
}
