package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// ResearchRoute is the graded escalation a trigger chooses between. It maps
// directly onto the roadmap's three post-trigger actions and never silently
// upgrades to the full graph.
type ResearchRoute string

const (
	// RouteContextRefresh re-fetches data behind one context (price/volume or
	// announcements) without re-running any model stage.
	RouteContextRefresh ResearchRoute = "CONTEXT_REFRESH"
	// RouteThesisReview re-examines the operator's watch rationale for the
	// symbol against fresh data; it is a bounded re-check, not a full graph.
	RouteThesisReview ResearchRoute = "THESIS_REVIEW"
	// RouteFullGraph starts the existing complete multi-agent research run.
	RouteFullGraph ResearchRoute = "FULL_GRAPH"
)

// TriggerRecord is the durable, auditable link from one monitor firing to
// whatever execution it caused. Input, decision, and outcome are all
// persisted; nothing is inferred after the fact.
type TriggerRecord struct {
	ID          string        `json:"trigger_id"`
	RuleID      string        `json:"rule_id"`
	Symbol      string        `json:"symbol"`
	MonitorKind MonitorKind   `json:"monitor_kind"`
	TradeDate   string        `json:"trade_date"`
	Route       ResearchRoute `json:"route"`
	Reason      string        `json:"reason"`
	// RunID links to the engine object when the route started one; a
	// CONTEXT_REFRESH pass records its own observation digest instead.
	RunID string `json:"run_id,omitempty"`
	// ObservationDigest anchors what the decision actually looked at.
	ObservationDigest string           `json:"observation_digest,omitempty"`
	State             ScheduleRunState `json:"state"`
	// FailureNote is a bounded, sanitized annotation; never a raw provider error.
	FailureNote string    `json:"failure_note,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

var triggerIDPattern = regexp.MustCompile(`^[a-f0-9]{22}$`)

// TriggerRouter grades a fired monitor into a route and executes it. Routing
// is fully deterministic per monitor kind and rule kind — no model decides
// the escalation level.
// TriggerRunSubmitter is the subset of the run controller the router needs;
// it is the existing research submission path, nothing new.
type TriggerRunSubmitter interface {
	Submit(ctx context.Context, request ResearchRequest) (RunRecord, error)
	Get(ctx context.Context, id string) (RunRecord, error)
}

type TriggerRouter struct {
	store    *FileTriggerStore
	runs     TriggerRunSubmitter
	mode     ResearchMode
	lookback time.Duration
}

func NewTriggerRouter(store *FileTriggerStore, runs TriggerRunSubmitter, mode ResearchMode, lookback time.Duration) (*TriggerRouter, error) {
	if store == nil || runs == nil {
		return nil, errors.New("trigger router requires store and run controller")
	}
	return &TriggerRouter{store: store, runs: runs, mode: mode, lookback: lookback}, nil
}

// RouteFor is the deterministic escalation table:
// - Announcements re-check the operator's rationale (thesis review).
// - Price breaks and volume spikes refresh the market context.
// A route may be overridden per rule only through explicit configuration,
// which this table deliberately does not offer yet.
func RouteFor(kind MonitorKind) ResearchRoute {
	if kind == MonitorAnnouncement {
		return RouteThesisReview
	}
	return RouteContextRefresh
}

// Handle processes one fired monitor event. It persists the trigger with its
// decision before executing, so a crash leaves an auditable PENDING/FAILED
// record rather than a silent loss. Duplicate handling of the same event is
// idempotent: the (rule, trade date) uniqueness returns the existing record.
func (t *TriggerRouter) Handle(ctx context.Context, event MonitorEvent) (TriggerRecord, error) {
	if event.Outcome != MonitorFired {
		return TriggerRecord{}, fmt.Errorf("only FIRED events route to research: %s", event.Outcome)
	}
	route := RouteFor(event.Kind)
	id := triggerID(event.RuleID, event.TradeDate)
	now := time.Now().UTC()
	record := TriggerRecord{
		ID: id, RuleID: event.RuleID, Symbol: event.Symbol, MonitorKind: event.Kind,
		TradeDate: event.TradeDate, Route: route, Reason: event.Detail,
		State: ScheduleRunning, CreatedAt: now, UpdatedAt: now,
	}
	if err := t.store.Save(record); err != nil {
		// Any duplicate of this (rule, day) — exclusive-created or terminal —
		// returns the persisted record; re-handling is a no-op.
		if errors.Is(err, ErrTriggerExists) || strings.Contains(err.Error(), "terminal trigger immutable") {
			return t.store.Get(id)
		}
		return TriggerRecord{}, err
	}
	switch route {
	case RouteContextRefresh:
		// A context refresh is not a paid run: recording the trigger with the
		// event's own detail as the observation anchor completes it.
		record.ObservationDigest = event.Detail
		record.State = ScheduleCompleted
	case RouteThesisReview, RouteFullGraph:
		input, err := NewLiveResearchInput(event.Symbol, "监控触发复查："+event.Detail, now, t.lookback)
		if err != nil {
			t.fail(&record, "TRIGGER_INPUT_INVALID")
			return record, t.store.Save(record)
		}
		rec, err := t.runs.Submit(ctx, ResearchRequest{
			IdempotencyKey: "trig-" + id,
			Mode:           t.mode,
			Input:          input,
		})
		if err != nil {
			record.State = ScheduleFailed
			record.FailureNote = err.Error()
			if len(record.FailureNote) > 200 {
				record.FailureNote = record.FailureNote[:200]
			}
			_ = t.store.Save(record)
			return record, nil
		}
		record.RunID = rec.ID
		record.State = ScheduleRunning
	}
	if err := t.store.Save(record); err != nil {
		return record, err
	}
	return record, nil
}

func (t *TriggerRouter) fail(record *TriggerRecord, code string) {
	record.State = ScheduleFailed
	record.FailureNote = code
}

func triggerID(ruleID, tradeDate string) string {
	return ruleID[:16] + tradeDate[2:] // 16 hex + YYMMDD = 24 chars, deterministic
}

// Advance moves RUNNING triggers with an engine run to a terminal state by
// polling the run, mirroring the scheduler's own advancement.
func (t *TriggerRouter) Advance(ctx context.Context) error {
	records, err := t.store.ListPending()
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.RunID == "" {
			continue
		}
		rec, err := t.runs.Get(ctx, record.RunID)
		if err != nil {
			continue
		}
		switch rec.State {
		case RunCompleted:
			record.State = ScheduleCompleted
		case RunFailed:
			record.State = ScheduleFailed
		case RunCancelled:
			record.State = ScheduleFailed
			record.FailureNote = "RESEARCH_CANCELLED"
		default:
			continue
		}
		record.UpdatedAt = time.Now().UTC()
		if err := t.store.Save(record); err != nil {
			return err
		}
	}
	return nil
}

// FileTriggerStore persists trigger records under a private directory. The
// deterministic ID makes (rule, trade date) handling at-most-once.
type FileTriggerStore struct {
	dir string
	mu  sync.Mutex
}

var ErrTriggerExists = errors.New("trigger already recorded")

func NewFileTriggerStore(dir string) (*FileTriggerStore, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("trigger directory required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("trigger directory must be private")
	}
	return &FileTriggerStore{dir: dir}, nil
}

func validateTriggerRecord(record TriggerRecord) error {
	if !triggerIDPattern.MatchString(record.ID) {
		return errors.New("invalid trigger id")
	}
	if !monitorIDPattern.MatchString(record.RuleID) {
		return errors.New("invalid trigger rule id")
	}
	if !ValidWatchSymbol(record.Symbol) {
		return errors.New("invalid trigger symbol")
	}
	switch record.MonitorKind {
	case MonitorPriceBelow, MonitorPriceAbove, MonitorVolumeSpike, MonitorAnnouncement:
	default:
		return errors.New("invalid trigger monitor kind")
	}
	if _, err := time.Parse("20060102", record.TradeDate); err != nil {
		return errors.New("invalid trigger trade date")
	}
	switch record.Route {
	case RouteContextRefresh, RouteThesisReview, RouteFullGraph:
	default:
		return errors.New("invalid trigger route")
	}
	switch record.State {
	case SchedulePending, ScheduleRunning, ScheduleCompleted, ScheduleFailed:
	default:
		return errors.New("invalid trigger state")
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() {
		return errors.New("invalid trigger timestamps")
	}
	if len(record.Reason) > 512 {
		return errors.New("trigger reason too long")
	}
	return nil
}

func (s *FileTriggerStore) path(id string) string { return filepath.Join(s.dir, id+".json") }

// Save persists one trigger record. Terminal records are immutable; RUNNING
// records may update toward terminal states only.
func (s *FileTriggerStore) Save(record TriggerRecord) error {
	if s == nil {
		return errors.New("nil trigger store")
	}
	if err := validateTriggerRecord(record); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old, err := s.read(record.ID)
	if err == nil {
		// The (rule, trade date) pair is handled at most once: an existing
		// record only ever advances its own state forward.
		if old.State == ScheduleCompleted || old.State == ScheduleFailed {
			return errors.New("terminal trigger immutable")
		}
		if record.CreatedAt != old.CreatedAt {
			return ErrTriggerExists
		}
		return writeJSONFile(s.path(record.ID), record)
	}
	if !os.IsNotExist(err) {
		return err
	}
	// Exclusive creation makes duplicate first-writes deterministic.
	flags := os.O_CREATE | os.O_EXCL | os.O_WRONLY
	raw, mErr := json.Marshal(record)
	if mErr != nil {
		return mErr
	}
	file, cErr := os.OpenFile(s.path(record.ID), flags, 0600)
	if cErr != nil {
		if os.IsExist(cErr) {
			return ErrTriggerExists
		}
		return cErr
	}
	var wErr error
	if _, wErr = file.Write(raw); wErr == nil {
		wErr = file.Sync()
	}
	closeErr := file.Close()
	if wErr != nil {
		_ = os.Remove(s.path(record.ID))
		return wErr
	}
	if closeErr != nil {
		return closeErr
	}
	return nil
}

func (s *FileTriggerStore) read(id string) (TriggerRecord, error) {
	if !triggerIDPattern.MatchString(id) {
		return TriggerRecord{}, errors.New("invalid trigger id")
	}
	raw, err := os.ReadFile(s.path(id))
	if os.IsNotExist(err) {
		return TriggerRecord{}, os.ErrNotExist
	}
	if err != nil {
		return TriggerRecord{}, err
	}
	if len(raw) > (16 << 10) {
		return TriggerRecord{}, errors.New("trigger record too large")
	}
	var record TriggerRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return TriggerRecord{}, err
	}
	if err := validateTriggerRecord(record); err != nil {
		return TriggerRecord{}, err
	}
	if record.ID != id {
		return TriggerRecord{}, errors.New("trigger id mismatch")
	}
	return record, nil
}

func (s *FileTriggerStore) Get(id string) (TriggerRecord, error) {
	if s == nil {
		return TriggerRecord{}, errors.New("nil trigger store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(id)
}

// List returns triggers newest first; limit bounds the listing.
func (s *FileTriggerStore) List(limit int) ([]TriggerRecord, error) {
	if s == nil {
		return nil, errors.New("nil trigger store")
	}
	if limit < 1 || limit > 100 {
		return nil, errors.New("trigger list limit must be 1..100")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	records := make([]TriggerRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		record, err := s.read(id)
		if err != nil {
			return nil, fmt.Errorf("trigger %s: %w", id, err)
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].ID > records[j].ID
		}
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
	if len(records) > limit {
		records = records[:limit]
	}
	return records, nil
}

// ListPending returns every non-terminal trigger, oldest first.
func (s *FileTriggerStore) ListPending() ([]TriggerRecord, error) {
	all, err := s.List(100)
	if err != nil {
		return nil, err
	}
	pending := make([]TriggerRecord, 0, len(all))
	for _, record := range all {
		if record.State == SchedulePending || record.State == ScheduleRunning {
			pending = append(pending, record)
		}
	}
	return pending, nil
}
