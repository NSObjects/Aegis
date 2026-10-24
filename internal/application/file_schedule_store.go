package application

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ScheduleKind selects which existing engine a task drives: the offline
// pullback scan or a full LLM research run.
type ScheduleKind string

const (
	ScheduleKindScan     ScheduleKind = "SCAN"
	ScheduleKindResearch ScheduleKind = "RESEARCH"
)

// ScheduleRunState is the persisted lifecycle of one (task, trading-day)
// execution. INTERRUPTED marks a RUNNING record orphaned by a process exit:
// it is never silently re-triggered.
type ScheduleRunState string

const (
	SchedulePending     ScheduleRunState = "PENDING"
	ScheduleRunning     ScheduleRunState = "RUNNING"
	ScheduleCompleted   ScheduleRunState = "COMPLETED"
	ScheduleFailed      ScheduleRunState = "FAILED"
	ScheduleInterrupted ScheduleRunState = "INTERRUPTED"
)

// ScheduleTask is one operator-defined recurring job. It references only
// existing engines; the scheduler never builds a second research path.
type ScheduleTask struct {
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	Kind           ScheduleKind `json:"kind"`
	Symbol         string       `json:"symbol,omitempty"`
	UniverseSource string       `json:"universe_source,omitempty"`
	Symbols        []string     `json:"symbols,omitempty"`
	Enabled        bool         `json:"enabled"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// ScheduleRun records one execution attempt keyed by (task, trading day).
// The pair is the idempotency boundary: once a record exists for the day the
// scheduler never triggers the task again, whatever the outcome.
type ScheduleRun struct {
	TaskID      string           `json:"task_id"`
	TradeDate   string           `json:"trade_date"`
	State       ScheduleRunState `json:"state"`
	RefID       string           `json:"ref_id,omitempty"`
	FailureCode string           `json:"failure_code,omitempty"`
	StartedAt   time.Time        `json:"started_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

var (
	scheduleIDPattern = regexp.MustCompile(`^[a-f0-9]{16}$`)
	scheduleDateCheck = regexp.MustCompile(`^[0-9]{8}$`)
	scheduleNameCheck = regexp.MustCompile(`^[\p{Han}\p{L}\p{N} _.-]{1,128}$`)
)

func ValidateScheduleTask(task ScheduleTask) error {
	if !scheduleIDPattern.MatchString(task.ID) {
		return errors.New("invalid schedule task id")
	}
	if !scheduleNameCheck.MatchString(task.Name) {
		return errors.New("invalid schedule task name")
	}
	if task.CreatedAt.IsZero() || task.UpdatedAt.IsZero() || task.UpdatedAt.Before(task.CreatedAt) {
		return errors.New("invalid schedule timestamps")
	}
	switch task.Kind {
	case ScheduleKindScan:
		if task.UniverseSource == "" {
			return errors.New("scan schedule requires universe source")
		}
		if len(task.Symbols) == 0 || len(task.Symbols) > 20 {
			return errors.New("scan schedule requires 1..20 symbols")
		}
		seen := map[string]bool{}
		for _, s := range task.Symbols {
			if !ValidWatchSymbol(s) || seen[s] {
				return errors.New("invalid scan schedule symbol")
			}
			seen[s] = true
		}
		if task.Symbol != "" {
			return errors.New("scan schedule must not carry a single symbol")
		}
	case ScheduleKindResearch:
		if !ValidWatchSymbol(task.Symbol) {
			return errors.New("research schedule requires a valid symbol")
		}
		if len(task.Symbols) != 0 || task.UniverseSource != "" {
			return errors.New("research schedule carries scan-only fields")
		}
	default:
		return errors.New("invalid schedule kind")
	}
	return nil
}

func validateScheduleRun(run ScheduleRun) error {
	if !scheduleIDPattern.MatchString(run.TaskID) {
		return errors.New("invalid schedule run task id")
	}
	if !scheduleDateCheck.MatchString(run.TradeDate) {
		if _, err := time.Parse("20060102", run.TradeDate); err != nil {
			return errors.New("invalid schedule run trade date")
		}
	}
	if run.StartedAt.IsZero() || run.UpdatedAt.IsZero() || run.UpdatedAt.Before(run.StartedAt) {
		return errors.New("invalid schedule run timestamps")
	}
	switch run.State {
	case SchedulePending, ScheduleRunning:
		if run.FailureCode != "" {
			return errors.New("unfinished schedule run has failure code")
		}
	case ScheduleCompleted:
		if run.RefID == "" || run.FailureCode != "" {
			return errors.New("completed schedule run requires ref")
		}
	case ScheduleFailed:
		if run.FailureCode == "" || len(run.FailureCode) > 64 {
			return errors.New("failed schedule run requires code")
		}
	case ScheduleInterrupted:
		if run.FailureCode != "PROCESS_INTERRUPTED" {
			return errors.New("interrupted run requires interruption code")
		}
	default:
		return errors.New("invalid schedule run state")
	}
	return nil
}

// FileScheduleStore persists task definitions and execution records under a
// private directory. Corrupt records fail closed; terminal runs stay mutable
// only in state-neutral ways (nothing rewrites history here).
type FileScheduleStore struct {
	dir string
	mu  sync.Mutex
}

func NewFileScheduleStore(dir string) (*FileScheduleStore, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("schedule directory required")
	}
	if err := os.MkdirAll(filepath.Join(dir, "tasks"), 0700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "runs"), 0700); err != nil {
		return nil, err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("schedule directory must be private")
	}
	return &FileScheduleStore{dir: dir}, nil
}

func NewScheduleTaskID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func writeJSONFile(path string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".schedule-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(raw)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("write schedule file: %w", err)
	}
	return nil
}

func (s *FileScheduleStore) taskPath(id string) string {
	return filepath.Join(s.dir, "tasks", id+".json")
}
func (s *FileScheduleStore) runPath(taskID, tradeDate string) string {
	return filepath.Join(s.dir, "runs", taskID+"_"+tradeDate+".json")
}

// SaveTask creates or replaces the definition. Replacements keep the ID and
// must stay the same kind: a task never mutates into a different engine.
func (s *FileScheduleStore) SaveTask(task ScheduleTask) error {
	if s == nil {
		return errors.New("nil schedule store")
	}
	if err := ValidateScheduleTask(task); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, err := s.readTask(task.ID); err == nil {
		if old.Kind != task.Kind {
			return errors.New("schedule task kind is immutable")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return writeJSONFile(s.taskPath(task.ID), task)
}

func (s *FileScheduleStore) readTask(id string) (ScheduleTask, error) {
	if !scheduleIDPattern.MatchString(id) {
		return ScheduleTask{}, errors.New("invalid schedule task id")
	}
	raw, err := os.ReadFile(s.taskPath(id))
	if err != nil {
		return ScheduleTask{}, err
	}
	if len(raw) > (16 << 10) {
		return ScheduleTask{}, errors.New("schedule task too large")
	}
	var task ScheduleTask
	if err := json.Unmarshal(raw, &task); err != nil {
		return ScheduleTask{}, err
	}
	if err := ValidateScheduleTask(task); err != nil {
		return ScheduleTask{}, err
	}
	if task.ID != id {
		return ScheduleTask{}, fmt.Errorf("schedule task id mismatch %s", id)
	}
	return task, nil
}

func (s *FileScheduleStore) GetTask(id string) (ScheduleTask, error) {
	if s == nil {
		return ScheduleTask{}, errors.New("nil schedule store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readTask(id)
}

func (s *FileScheduleStore) ListTasks() ([]ScheduleTask, error) {
	if s == nil {
		return nil, errors.New("nil schedule store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(filepath.Join(s.dir, "tasks"))
	if err != nil {
		return nil, err
	}
	tasks := make([]ScheduleTask, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		task, err := s.readTask(id)
		if err != nil {
			return nil, fmt.Errorf("schedule task %s: %w", id, err)
		}
		tasks = append(tasks, task)
	}
	sortScheduleTasks(tasks)
	return tasks, nil
}

func sortScheduleTasks(tasks []ScheduleTask) {
	for i := 1; i < len(tasks); i++ {
		for j := i; j > 0 && tasks[j].ID < tasks[j-1].ID; j-- {
			tasks[j], tasks[j-1] = tasks[j-1], tasks[j]
		}
	}
}

// GetRun returns the persisted execution record for one (task, trading day);
// os.ErrNotExist when the scheduler has not triggered it.
func (s *FileScheduleStore) GetRun(taskID, tradeDate string) (ScheduleRun, error) {
	if s == nil {
		return ScheduleRun{}, errors.New("nil schedule store")
	}
	if !scheduleIDPattern.MatchString(taskID) {
		return ScheduleRun{}, errors.New("invalid schedule run task id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readRun(taskID, tradeDate)
}

func (s *FileScheduleStore) readRun(taskID, tradeDate string) (ScheduleRun, error) {
	raw, err := os.ReadFile(s.runPath(taskID, tradeDate))
	if err != nil {
		return ScheduleRun{}, err
	}
	if len(raw) > (8 << 10) {
		return ScheduleRun{}, errors.New("schedule run too large")
	}
	var run ScheduleRun
	if err := json.Unmarshal(raw, &run); err != nil {
		return ScheduleRun{}, err
	}
	if err := validateScheduleRun(run); err != nil {
		return ScheduleRun{}, err
	}
	if run.TaskID != taskID || run.TradeDate != tradeDate {
		return ScheduleRun{}, errors.New("schedule run identity mismatch")
	}
	return run, nil
}

// SaveRun persists one execution-record transition. State may only move
// forward; a record for a day is never deleted or reset by the scheduler.
func (s *FileScheduleStore) SaveRun(run ScheduleRun) error {
	if s == nil {
		return errors.New("nil schedule store")
	}
	if err := validateScheduleRun(run); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old, err := s.readRun(run.TaskID, run.TradeDate)
	if err == nil && scheduleRank(old.State) > scheduleRank(run.State) {
		return errors.New("schedule run state moved backwards")
	}
	return writeJSONFile(s.runPath(run.TaskID, run.TradeDate), run)
}

func scheduleRank(state ScheduleRunState) int {
	switch state {
	case SchedulePending:
		return 0
	case ScheduleRunning:
		return 1
	case ScheduleCompleted, ScheduleFailed, ScheduleInterrupted:
		return 2
	}
	return -1
}

// ReconcileInterrupted marks RUNNING records from a previous process as
// INTERRUPTED at startup. They stay visible and are never re-triggered for
// the same trading day: a crashed paid research must not silently restart.
func (s *FileScheduleStore) ReconcileInterrupted() (int, error) {
	if s == nil {
		return 0, errors.New("nil schedule store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(filepath.Join(s.dir, "runs"))
	if err != nil {
		return 0, err
	}
	changed := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		base := strings.TrimSuffix(entry.Name(), ".json")
		sep := strings.LastIndex(base, "_")
		if sep <= 0 {
			return changed, fmt.Errorf("invalid schedule run filename %q", entry.Name())
		}
		taskID, tradeDate := base[:sep], base[sep+1:]
		run, err := s.readRun(taskID, tradeDate)
		if err != nil {
			return changed, fmt.Errorf("schedule run %s: %w", base, err)
		}
		if run.State != ScheduleRunning {
			continue
		}
		run.State = ScheduleInterrupted
		run.FailureCode = "PROCESS_INTERRUPTED"
		run.UpdatedAt = time.Now().UTC()
		if run.UpdatedAt.Before(run.StartedAt) {
			run.UpdatedAt = run.StartedAt
		}
		if err := writeJSONFile(s.runPath(taskID, tradeDate), run); err != nil {
			return changed, err
		}
		changed++
	}
	return changed, nil
}

// DeleteTask removes a task definition. Enabled tasks must be disabled
// first, so a live daily trigger cannot vanish invisibly. Past execution
// records are kept as history.
func (s *FileScheduleStore) DeleteTask(id string) error {
	if s == nil {
		return errors.New("nil schedule store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	task, err := s.readTask(id)
	if err != nil {
		return err
	}
	if task.Enabled {
		return errors.New("disable the task before deleting it")
	}
	return os.Remove(s.taskPath(id))
}

// ListRuns returns every persisted execution record for one task, oldest
// first. Corrupt records fail the listing.
func (s *FileScheduleStore) ListRuns(taskID string) ([]ScheduleRun, error) {
	if s == nil {
		return nil, errors.New("nil schedule store")
	}
	if !scheduleIDPattern.MatchString(taskID) {
		return nil, errors.New("invalid schedule run task id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(filepath.Join(s.dir, "runs"))
	if err != nil {
		return nil, err
	}
	runs := make([]ScheduleRun, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		base := strings.TrimSuffix(entry.Name(), ".json")
		sep := strings.LastIndex(base, "_")
		if sep <= 0 || base[:sep] != taskID {
			continue
		}
		run, err := s.readRun(taskID, base[sep+1:])
		if err != nil {
			return nil, fmt.Errorf("schedule run %s: %w", base, err)
		}
		runs = append(runs, run)
	}
	for i := 1; i < len(runs); i++ {
		for j := i; j > 0 && runs[j].TradeDate < runs[j-1].TradeDate; j-- {
			runs[j], runs[j-1] = runs[j-1], runs[j]
		}
	}
	return runs, nil
}

// CreateTask generates nothing itself; the HTTP layer assigns the ID and
// timestamps. It exists so the controller surface stays explicit.
func (s *FileScheduleStore) CreateTask(task ScheduleTask) (ScheduleTask, error) {
	if err := s.SaveTask(task); err != nil {
		return ScheduleTask{}, err
	}
	return task, nil
}

// UpdateTask replaces a stored definition wholesale (same immutable rules).
func (s *FileScheduleStore) UpdateTask(task ScheduleTask) error { return s.SaveTask(task) }
