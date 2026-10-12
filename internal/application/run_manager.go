package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"Aegis/internal/research"
 eventcap "Aegis/internal/capability/event"
 technicalcap "Aegis/internal/capability/technical"
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
	FailureTimeout       = "RESEARCH_TIMEOUT"
)

// RunEvent is a persisted, replayable, non-sensitive execution event.
// Seq increases strictly within a run; clients resume with Last-Event-ID.
type RunEvent struct {
	// Kind is omitted for historical stage events; TOOL means observed invocation metadata.
	Kind string `json:"kind,omitempty"`
	Tool string `json:"tool,omitempty"`
	Outcome string `json:"outcome,omitempty"`
	Seq        uint64    `json:"seq"`
	Stage      string    `json:"stage"`
	Status     string    `json:"status"`
	At         time.Time `json:"at"`
	DurationMS int64     `json:"duration_ms,omitempty"`
}

// RunRequestSnapshot is a small, non-sensitive, immutable projection of the
// submitted research input. It does not record query text, account holdings,
// credentials, model parameters or the actual sources eventually retrieved.
// SchemaVersion is this snapshot's schema, not a model/provider version.
type RunRequestSnapshot struct {
	SchemaVersion int `json:"schema_version"`
	MarketQueryProvided bool `json:"market_query_provided"`
	EventWindowStart *time.Time `json:"event_window_start,omitempty"`
	EventWindowEnd *time.Time `json:"event_window_end,omitempty"`
	EventPITSafe bool `json:"event_pit_safe"`
	PreferredSourceTypes []string `json:"preferred_source_types,omitempty"`
	MinDocuments int `json:"min_documents,omitempty"`
	MinIndependentSources int `json:"min_independent_sources,omitempty"`
	PortfolioState string `json:"portfolio_state,omitempty"`
}

// snapshotRunRequest runs on the already JSON-frozen input. Only individually
// allowlisted fields may be persisted; never persist the original request.
func snapshotRunRequest(req ResearchRequest) *RunRequestSnapshot {
	task := req.Input.EventInput.Task
	meta := &RunRequestSnapshot{
		SchemaVersion: 1,
		MarketQueryProvided: strings.TrimSpace(req.Input.MarketQuery) != "",
		EventPITSafe: task.Requirements.PITSafe,
		MinDocuments: task.Requirements.MinDocuments,
		MinIndependentSources: task.Requirements.MinIndependentSources,
	}
	if !task.TimeRange.Start.IsZero() && !task.TimeRange.End.IsZero() {
		start, end := task.TimeRange.Start, task.TimeRange.End
		meta.EventWindowStart, meta.EventWindowEnd = &start, &end
	}
	for _, source := range task.Requirements.PreferredSourceTypes {
		meta.PreferredSourceTypes = append(meta.PreferredSourceTypes, string(source))
	}
	if req.Portfolio != nil { meta.PortfolioState = string(req.Portfolio.State) }
	return meta
}

// RunRecord stores metadata and exact typed workflow JSON, never a raw input
// portfolio snapshot. Artifact exists only for completed runs.
type RunRecord struct {
	IdempotencyDigest string `json:"idempotency_digest,omitempty"`
	RequestFingerprint string `json:"request_fingerprint,omitempty"`
	ID            string          `json:"run_id"`
	Mode          ResearchMode    `json:"mode"`
	Symbol        string          `json:"symbol"`
	RerunOf       string          `json:"rerun_of,omitempty"`
	RequestSnapshot *RunRequestSnapshot `json:"request_snapshot,omitempty"`
	// Present only if the live model graph was constructed successfully.
	ModelProvenance *RunModelProvenance `json:"model_provenance,omitempty"`
	// Present on completed Portfolio Review only when its EventContext exists.
	EvidenceIndex *RunEvidenceIndex `json:"evidence_index,omitempty"`
	RequestedAsOf time.Time       `json:"requested_as_of"`
	State         RunState        `json:"state"`
	CreatedAt     time.Time       `json:"created_at"`
	StartedAt     *time.Time      `json:"started_at,omitempty"`
	FinishedAt    *time.Time      `json:"finished_at,omitempty"`
	FailureCode   string          `json:"failure_code,omitempty"`
	// FailureStage is set only when a Go stage emitted an explicit FAIL event.
	FailureStage string `json:"failure_stage,omitempty"`
	// FailureCategory is a safe, fixed identifier, never a raw provider error.
	FailureCategory string `json:"failure_category,omitempty"`
	Artifact      json.RawMessage `json:"artifact,omitempty"`
	// ArtifactVersion is the version of Aegis's persisted result envelope,
	// not a claim about the provider's model version.
	// Zero identifies pre-R1-03 legacy run records.
	ArtifactVersion int `json:"artifact_version,omitempty"`
	ArtifactSHA256 string `json:"artifact_sha256,omitempty"`
	HistoricalOHLCV *technicalcap.HistoricalObservation `json:"historical_ohlcv,omitempty"`
	Events        []RunEvent      `json:"events,omitempty"`
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
	ErrRunIdempotencyConflict = errors.New("idempotency key already used for a different research request")
	ErrInvalidRerunSource = errors.New("invalid rerun source")
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
type runKeyIndexEntry struct {
	RunID string
	Fingerprint string
}

type RunManager struct {
	executor  ResearchExecutor
	store     RunStore
	mu        sync.Mutex
	active    map[string]context.CancelFunc
	idempotency map[string]runKeyIndexEntry
	closed    bool
	ctx       context.Context
	cancel    context.CancelFunc
	slots     chan struct{}
	maxActive int
	runTimeout time.Duration
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
	index := make(map[string]runKeyIndexEntry)
	for _, record := range existing {
		if record.IdempotencyDigest != "" {
			if previous, exists := index[record.IdempotencyDigest]; exists {
				return nil, fmt.Errorf("duplicate persisted idempotency key for runs %s and %s", previous.RunID, record.ID)
			}
			index[record.IdempotencyDigest] = runKeyIndexEntry{RunID: record.ID, Fingerprint: record.RequestFingerprint}
		}
	}
	now := time.Now().UTC()
	for _, record := range existing {
		if record.State != RunPending && record.State != RunRunning {
			continue
		}
		record.State = RunFailed
		record.FinishedAt = &now
		record.FailureCode = FailureInterrupted
		record.FailureCategory = "PROCESS_INTERRUPTED"
		record.Artifact = nil
		record.Events = append(record.Events, RunEvent{Seq: uint64(len(record.Events) + 1), Stage: "Run", Status: string(RunFailed), At: now})
		if err := store.Save(context.Background(), record); err != nil {
			return nil, fmt.Errorf("reconcile interrupted run %s: %w", record.ID, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &RunManager{executor: executor, store: store, active: make(map[string]context.CancelFunc), idempotency: index, ctx: ctx, cancel: cancel, slots: make(chan struct{}, maxParallel), maxActive: maxActive, runTimeout: 20*time.Minute, now: func() time.Time { return time.Now().UTC() }}, nil
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
	key := request.IdempotencyKey
	if err := ValidateIdempotencyKey(key); err != nil {return RunRecord{}, err}
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
	requestSnapshot := snapshotRunRequest(request)
	var keyDigest, requestFingerprint string
	if key != "" {
		keyHash := sha256.Sum256([]byte(key))
		keyDigest = hex.EncodeToString(keyHash[:])
		var err error
		requestFingerprint, err = stableRequestFingerprint(request)
		if err != nil {return RunRecord{}, err}
	}
	id, err := secureRunID()
	if err != nil {
		return RunRecord{}, err
	}
	now := m.now()
	record := RunRecord{IdempotencyDigest:keyDigest, RequestFingerprint:requestFingerprint, ID: id, Mode: request.Mode, Symbol: strings.TrimSpace(request.Input.Symbol), RerunOf: request.RerunOf, RequestSnapshot: requestSnapshot, RequestedAsOf: request.Input.RequestedAsOf, State: RunPending, CreatedAt: now, Events: []RunEvent{{Seq: 1, Stage: "Run", Status: string(RunPending), At: now}}}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return RunRecord{}, ErrRunManagerClosed
	}
	if keyDigest != "" {
		if previous, exists := m.idempotency[keyDigest]; exists {
			if previous.Fingerprint != requestFingerprint {return RunRecord{}, ErrRunIdempotencyConflict}
			return m.store.Get(ctx, previous.RunID)
		}
	}
	// A new study must reference an existing terminal run of the same security and
	// mode. The previous request and account are never replayed from storage.
	if request.RerunOf != "" {
		parent, err := m.store.Get(ctx, request.RerunOf)
		if err != nil {
			if errors.Is(err, ErrRunNotFound) { return RunRecord{}, ErrInvalidRerunSource }
			return RunRecord{}, err
		}
		if (parent.State != RunCompleted && parent.State != RunFailed && parent.State != RunCancelled) || parent.Symbol != record.Symbol || parent.Mode != record.Mode {
			return RunRecord{}, ErrInvalidRerunSource
		}
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
	runCtx, cancel := context.WithTimeout(m.ctx, m.runTimeout)
	if keyDigest != "" {m.idempotency[keyDigest] = runKeyIndexEntry{RunID:id, Fingerprint:requestFingerprint}}
	m.active[id] = cancel
	m.wg.Add(1)
	go m.execute(runCtx, request, record, cancel)
	// Detach response metadata from the goroutine-owned persisted snapshot.
	// A caller may mutate its response without changing a running research task.
	returned := record
	if returned.RequestSnapshot != nil {
		copy := *returned.RequestSnapshot
		copy.PreferredSourceTypes = append([]string(nil), copy.PreferredSourceTypes...)
		returned.RequestSnapshot = &copy
	}
	returned.Events = append([]RunEvent(nil), record.Events...)
	return returned, nil
}

// ValidateIdempotencyKey rejects long or unsafe external request identifiers.
// The raw key is never persisted or emitted as part of a run.
func ValidateIdempotencyKey(key string) error {
	if key == "" {return nil}
	if len(key)>128 || len(key)<8 {return errors.New("Idempotency-Key must be 8-128 ASCII characters")}
	for _, c := range key {
		if (c>='a'&&c<='z')||(c>='A'&&c<='Z')||(c>='0'&&c<='9')||c=='-'||c=='_'||c=='.'||c==':' {continue}
		return errors.New("Idempotency-Key contains invalid characters")
	}
	return nil
}

// The API determines a fresh cutoff at submission. A retried HTTP request
// must reuse the original run even though the wall clock has changed.
// Exclude only generated cutoff timestamps; retain the evidence-window
// duration and all caller-authored research constraints and account facts.
func stableRequestFingerprint(request ResearchRequest) (string,error) {
	window := request.Input.EventInput.Task.TimeRange.End.Sub(request.Input.EventInput.Task.TimeRange.Start)
	canonical := request
	canonical.IdempotencyKey = ""
	canonical.Input.RequestedAsOf = time.Time{}
	canonical.Input.EventInput.Task.AsOf = time.Time{}
	canonical.Input.EventInput.Task.TimeRange.Start = time.Time{}
	canonical.Input.EventInput.Task.TimeRange.End = time.Time{}
	encoded,err := json.Marshal(struct{
		Request ResearchRequest `json:"request"`
		EventWindowNanos int64 `json:"event_window_nanos"`
	}{Request:canonical,EventWindowNanos:int64(window)})
	if err!=nil {return "",fmt.Errorf("fingerprint research request: %w",err)}
	h := sha256.Sum256(encoded)
	return hex.EncodeToString(h[:]),nil
}

func validateRunRequest(req ResearchRequest) error {
	if req.RerunOf != "" && !validRunID(req.RerunOf) { return ErrInvalidRerunSource }
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

// artifactDigest uses canonical JSON bytes so changes in insignificant
// whitespace cannot invalidate a previously persisted result.
func artifactDigest(raw json.RawMessage) string {
	var canonical strings.Builder
	// json.Marshal compacts valid RawMessage and retains field ordering.
	compact, err := json.Marshal(raw)
	if err != nil { return "" }
	canonical.Write(compact)
	sum := sha256.Sum256([]byte(canonical.String()))
	return hex.EncodeToString(sum[:])
}

func secureRunID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate run ID: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func (m *RunManager) execute(ctx context.Context, request ResearchRequest, record RunRecord, cancel context.CancelFunc) {
	defer cancel()
	defer m.wg.Done()
	defer func() { m.mu.Lock(); delete(m.active, record.ID); m.mu.Unlock() }()

	// Workflow stage callbacks may arrive from parallel agents. Protect the
	// authoritative snapshot while persisting each event atomically.
	var mu sync.Mutex
	var eventPersistError bool
	var completedIndex *RunEvidenceIndex
	appendEvent := func(stage, status string, at time.Time, durationMS int64) error {
		record.Events = append(record.Events, RunEvent{Seq: uint64(len(record.Events) + 1), Stage: stage, Status: status, At: at, DurationMS: durationMS})
		return m.store.Save(context.Background(), record)
	}
	terminal := func(state RunState, code string, output json.RawMessage) {
		mu.Lock()
		defer mu.Unlock()
		if record.State == RunCompleted || record.State == RunFailed || record.State == RunCancelled {
			return
		}
		done := m.now()
		record.State = state
		record.FinishedAt = &done
		record.Artifact = output
		if state == RunCompleted {
			record.EvidenceIndex = completedIndex
			record.ArtifactVersion = 1
			record.ArtifactSHA256 = artifactDigest(output)
		} else {
			record.EvidenceIndex = nil
			record.ArtifactVersion = 0
			record.ArtifactSHA256 = ""
		}
		record.FailureCode = code
		if state == RunFailed {
			record.FailureStage = explicitFailedStage(record.Events)
			switch code {
			case FailurePersistence: record.FailureCategory = "PERSISTENCE"
			case FailureTimeout: record.FailureCategory = "TIMEOUT"
			case FailureInvalidResult: record.FailureCategory = "INVALID_RESULT"
			case FailureInterrupted: record.FailureCategory = "PROCESS_INTERRUPTED"
			default: if record.FailureCategory == "" { record.FailureCategory = "UNKNOWN" }
			}
		}
        if state != RunCompleted { record.HistoricalOHLCV = nil }
		// The store is authoritative. A failed disk write leaves prior state
		// for process-interrupted recovery and cannot claim completion.
		_ = appendEvent("Run", string(state), done, 0)
	}
	select {
	case m.slots <- struct{}{}:
		defer func() { <-m.slots }()
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) { terminal(RunFailed, FailureTimeout, nil) } else { terminal(RunCancelled, "", nil) }
		return
	}
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) { terminal(RunFailed, FailureTimeout, nil) } else { terminal(RunCancelled, "", nil) }
		return
	}
	mu.Lock()
	started := m.now()
	record.StartedAt = &started
	record.State = RunRunning
	err := appendEvent("Run", string(RunRunning), started, 0)
	mu.Unlock()
	if err != nil {
		terminal(RunFailed, FailurePersistence, nil)
		return
	}

	// Record actual graph construction metadata before model research starts.
	// The sink is scoped to this Run; concurrent Runs cannot cross-write state.
	ctx = withModelProvenanceSink(ctx, func(meta *RunModelProvenance) error {
		mu.Lock()
		defer mu.Unlock()
		if record.State != RunRunning || eventPersistError || record.ModelProvenance != nil {
			return errors.New("model identity cannot be recorded for this run")
		}
		if err := validateRunModelProvenance(meta); err != nil { return err }
		if meta == nil { return errors.New("model identity must not be nil") }
		cloned := *meta
		cloned.Bindings = append([]ModelRoleBinding(nil), meta.Bindings...)
		record.ModelProvenance = &cloned
		if err := m.store.Save(context.Background(), record); err != nil {
			record.ModelProvenance = nil
			eventPersistError = true
			cancel() // never continue paid research after persistence fails
			return err
		}
		return nil
	})
	ctx = research.WithToolObservations(ctx, func(ev research.ToolObservation) {
		mu.Lock()
		defer mu.Unlock()
		if record.State != RunRunning || eventPersistError { return }
		// A tool event belongs to its real stage, but is not a stage transition.
		record.Events = append(record.Events, RunEvent{
			Seq: uint64(len(record.Events) + 1), Kind: "TOOL",
			Stage: ev.Stage, Tool: ev.Tool, Status: ev.Status, Outcome: ev.Outcome,
			At: ev.At, DurationMS: ev.DurationMS,
		})
		if err := m.store.Save(context.Background(), record); err != nil {
			eventPersistError = true
			cancel()
		}
	})
    // Only actual calls observed by the acquisition runner produce events.
    // The common sink enforces identifier allowlisting before durable storage.
    ctx = technicalcap.WithHistoricalObserver(ctx, func(obs technicalcap.HistoricalObservation) {
        mu.Lock()
        defer mu.Unlock()
        if record.State != RunRunning || eventPersistError { return }
        if err := validateHistoricalObservation(record, &obs); err != nil { return }
        // An existing observation is immutable: retries may not silently replace it.
        if record.HistoricalOHLCV != nil { return }
        cloned := obs
        cloned.Bars = append([]technicalcap.HistoricalBar(nil), obs.Bars...)
        record.HistoricalOHLCV = &cloned
        if err := m.store.Save(context.Background(), record); err != nil {
            record.HistoricalOHLCV = nil
            eventPersistError = true
            cancel()
        }
    })
    ctx = technicalcap.WithToolObserver(ctx, func(ev technicalcap.ToolObservation) {
        research.ReportToolObservation(ctx, research.ToolObservation{
            Stage: "Technical", Tool: technicalcap.ToolName, Status: ev.Status,
            At: ev.At, DurationMS: ev.DurationMS,
        })
    })
    ctx = eventcap.WithRetrievalObserver(ctx, func(ev eventcap.RetrievalObservation) {
        research.ReportToolObservation(ctx, research.ToolObservation{
            Stage: "Event", Tool: ev.RetrieverID, Status: ev.Status, Outcome: ev.Outcome,
            At: ev.At, DurationMS: ev.DurationMS,
        })
    })
	ctx = research.WithProgressEvents(ctx, func(ev research.ProgressEvent) {
		mu.Lock()
		defer mu.Unlock()
		if record.State != RunRunning || eventPersistError {
			return
		}
		if err := appendEvent(ev.Stage, ev.Status, ev.At, ev.DurationMS); err != nil {
			eventPersistError = true
			// Avoid paying for further model work after event persistence fails.
			cancel()
		}
	})
	// A non-cooperative provider may ignore ctx and never return. Persist the
	// deadline/cancellation independently while retaining the execution slot
	// until the provider actually exits. Late callbacks/results are rejected
	// because terminal() switches the authoritative Run state under mu.
	watchdogDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				terminal(RunFailed, FailureTimeout, nil)
			} else {
				terminal(RunCancelled, "", nil)
			}
		case <-watchdogDone:
		}
	}()
	defer close(watchdogDone)
	response, err := m.executor.Run(ctx, request)
	mu.Lock()
	persistFailed := eventPersistError
	mu.Unlock()
	if persistFailed {
		terminal(RunFailed, FailurePersistence, nil)
		return
	}
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) { terminal(RunFailed, FailureTimeout, nil) } else { terminal(RunCancelled, "", nil) }
		return
	}
	if err != nil {
		// A diagnostic category is safe for hosted CI. Never log err.Error():
		// provider failures can embed prompts, response bodies or credentials.
		log.Printf("AEGIS_RUN_DIAGNOSTIC run=%s stage=%s category=%s", record.ID, lastResearchStage(record.Events), researchErrorCategory(err))
		mu.Lock()
		record.FailureCategory = researchErrorCategory(err)
		mu.Unlock()
		terminal(RunFailed, FailureResearch, nil)
		return
	}
	if response == nil || response.Mode != request.Mode || response.Output() == nil {
		terminal(RunFailed, FailureInvalidResult, nil)
		return
	}
	artifact, e := json.Marshal(response.Output())
	if e != nil || len(artifact) == 0 || string(artifact) == "null" {
		terminal(RunFailed, FailureInvalidResult, nil)
		return
	}
	index, e := projectRunEvidenceIndex(artifact, response.Mode, record.Symbol, record.RequestedAsOf)
	if e != nil {
		terminal(RunFailed, FailureInvalidResult, nil)
		return
	}
	completedIndex = index
	terminal(RunCompleted, "", artifact)
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


// researchErrorCategory deliberately emits only fixed identifiers. Unknown
// errors remain UNKNOWN rather than publishing LLM output or provider details.
func researchErrorCategory(err error) string {
	if err == nil { return "NONE" }
	msg := strings.ToLower(err.Error())
	switch {
	// Event categories are matched against stable wrapper context, never persisted raw text.
	// Tool completion alone is not evidence that the Event agent succeeded.
	case strings.Contains(msg, "event research:") && strings.Contains(msg, "decode strict json"):
		return "EVENT_JSON_DECODE"
	case strings.Contains(msg, "event research:") && strings.Contains(msg, "generate model output"):
		return "EVENT_MODEL_GENERATE"
	case strings.Contains(msg, "event research:") && strings.Contains(msg, "model returned empty output"):
		return "EVENT_MODEL_EMPTY"
	case strings.Contains(msg, "invalid event context:") || strings.Contains(msg, "validate event analyst context:"):
		return "EVENT_CONTEXT_VALIDATION"
	case strings.Contains(msg, "event research:") && strings.Contains(msg, "normalize event analyst synthesis"):
		return "EVENT_SYNTHESIS_VALIDATION"
	case strings.Contains(msg, "event research:") && (strings.Contains(msg, "run event evidence research loop:") || strings.Contains(msg, "create event evidence research loop:") || strings.Contains(msg, "event evidence research loop completed without") || strings.Contains(msg, "event evidence research returned task")):
		return "EVENT_EVIDENCE_LOOP"
	case strings.Contains(msg, "event research:") && (strings.Contains(msg, "capture live event evidence:") || strings.Contains(msg, "live event capture completed without")):
		return "EVENT_CAPTURE"
	case strings.Contains(msg, "event research:") && strings.Contains(msg, "analyze researched event evidence:"):
		return "EVENT_ANALYSIS"
	case strings.Contains(msg, "event research:") && (strings.Contains(msg, "index cited event sources:") || strings.Contains(msg, "validate cited event sources:")):
		return "EVENT_SOURCE_VALIDATION"
	case strings.Contains(msg, "technical agent returned empty output") || strings.Contains(msg, "technical context output is empty"):
		return "TECHNICAL_MODEL_EMPTY"
	case strings.Contains(msg, "invalid technical context json"):
		return "TECHNICAL_JSON_DECODE"
	case strings.Contains(msg, "invalid technical context:") || strings.Contains(msg, "technical context symbol") || strings.Contains(msg, "technical context requested_as_of"):
		return "TECHNICAL_CONTEXT_VALIDATION"
	case strings.Contains(msg, "market agent returned empty output") || strings.Contains(msg, "market context output is empty"):
		return "MARKET_MODEL_EMPTY"
	case strings.Contains(msg, "invalid market context json"):
		return "MARKET_JSON_DECODE"
	case strings.Contains(msg, "invalid market context after one correction") || strings.Contains(msg, "invalid market context:"):
		return "MARKET_CONTEXT_VALIDATION"
	case strings.Contains(msg, "research narrative is not in simplified chinese"):
		return "LANGUAGE_VALIDATION"
	case strings.Contains(msg, "decode research synthesis json"):
		return "SYNTHESIS_JSON_DECODE"
	case strings.Contains(msg, "invalid research synthesis artifact"):
		return "SYNTHESIS_SCHEMA_VALIDATION"
	case strings.Contains(msg, "generate research synthesis"):
		return "SYNTHESIS_MODEL_GENERATE"
	case strings.Contains(msg, "research synthesis model returned empty"):
		return "SYNTHESIS_EMPTY_OUTPUT"
	default:
		return "UNKNOWN"
	}
}

func lastResearchStage(events []RunEvent) string {
	for i := len(events)-1; i>=0; i-- {
		switch events[i].Stage {
		case "Market", "Technical", "Event", "Synthesis", "Debate", "Manager", "Trading", "Risk", "Portfolio":
			return events[i].Stage
		}
	}
	return "UNKNOWN"
}

// explicitFailedStage never infers a failure from a START without FAIL or a tool event.
func explicitFailedStage(events []RunEvent) string {
	for i := len(events)-1; i >= 0; i-- {
		ev := events[i]
		if ev.Kind == "" && ev.Status == "FAIL" && ev.Stage != "Run" {
			return ev.Stage
		}
	}
	return ""
}

// SetRunTimeout configures the deadline for future submissions. Call once
// during initialization, before concurrent Submit calls; zero is not allowed.
func (m *RunManager) SetRunTimeout(timeout time.Duration) error {
 if m == nil || timeout <= 0 { return errors.New("research timeout must be positive") }
 m.mu.Lock()
 defer m.mu.Unlock()
 if len(m.active) != 0 { return errors.New("cannot change timeout while research is active") }
 m.runTimeout = timeout
 return nil
}
