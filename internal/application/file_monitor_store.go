package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MonitorKind enumerates the deterministic watch conditions. Every check is
// computed from provider data; none invokes an LLM.
type MonitorKind string

const (
	MonitorPriceBelow   MonitorKind = "PRICE_BELOW"
	MonitorPriceAbove   MonitorKind = "PRICE_ABOVE"
	MonitorVolumeSpike  MonitorKind = "VOLUME_SPIKE"
	MonitorAnnouncement MonitorKind = "ANNOUNCEMENT"
)

// MonitorOutcome is the per-run result of one rule beyond a plain event:
// source failures are recorded as alarms, never folded into "no event".
type MonitorOutcome string

const (
	MonitorFired      MonitorOutcome = "FIRED"
	MonitorQuiet      MonitorOutcome = "QUIET"
	MonitorSuppressed MonitorOutcome = "COOLDOWN_SUPPRESSED"
	MonitorSourceDown MonitorOutcome = "SOURCE_UNAVAILABLE"
)

var monitorIDPattern = regexp.MustCompile(`^[a-f0-9]{16}$`)

// MonitorRule is one operator-defined watch condition over a symbol.
// Threshold semantics: PRICE_BELOW/PRICE_ABOVE compare the raw close against
// Threshold; VOLUME_SPIKE requires volume >= Threshold × the 5-session mean
// volume; ANNOUNCEMENT ignores Threshold and fires when the provider returns
// any announcement in the lookback window.
type MonitorRule struct {
	ID           string      `json:"id"`
	Symbol       string      `json:"symbol"`
	Kind         MonitorKind `json:"kind"`
	Threshold    float64     `json:"threshold,omitempty"`
	CooldownDays int         `json:"cooldown_days"`
	Enabled      bool        `json:"enabled"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

// MonitorEvent is one durable watch outcome. The (rule, trade date) pair is
// unique: a rule records at most one outcome per day, which is the dedup
// backbone; CooldownDays then bounds repetition across days.
type MonitorEvent struct {
	RuleID    string         `json:"rule_id"`
	Symbol    string         `json:"symbol"`
	Kind      MonitorKind    `json:"kind"`
	TradeDate string         `json:"trade_date"`
	Outcome   MonitorOutcome `json:"outcome"`
	Detail    string         `json:"detail"`
	CreatedAt time.Time      `json:"created_at"`
}

// MonitorEventLimit caps how many FIRED events one rule may accumulate per
// trading day at the store level — the budget backstop behind the scheduler's
// once-per-day trigger.
const MonitorEventLimit = 50

func validateMonitorRule(rule MonitorRule) error {
	if !monitorIDPattern.MatchString(rule.ID) {
		return errors.New("invalid monitor rule id")
	}
	if !ValidWatchSymbol(rule.Symbol) {
		return errors.New("invalid monitor rule symbol")
	}
	switch rule.Kind {
	case MonitorPriceBelow, MonitorPriceAbove:
		if rule.Threshold <= 0 {
			return errors.New("price monitor requires a positive threshold")
		}
	case MonitorVolumeSpike:
		if rule.Threshold <= 0 {
			return errors.New("volume spike requires a positive multiple")
		}
	case MonitorAnnouncement:
		if rule.Threshold != 0 {
			return errors.New("announcement monitor takes no threshold")
		}
	default:
		return errors.New("invalid monitor kind")
	}
	if rule.CooldownDays < 0 || rule.CooldownDays > 365 {
		return errors.New("invalid cooldown")
	}
	if rule.CreatedAt.IsZero() || rule.UpdatedAt.IsZero() || rule.UpdatedAt.Before(rule.CreatedAt) {
		return errors.New("invalid monitor timestamps")
	}
	return nil
}

func validateMonitorEvent(event MonitorEvent) error {
	if !monitorIDPattern.MatchString(event.RuleID) {
		return errors.New("invalid monitor event rule id")
	}
	if !ValidWatchSymbol(event.Symbol) {
		return errors.New("invalid monitor event symbol")
	}
	if _, err := time.Parse("20060102", event.TradeDate); err != nil || len(event.TradeDate) != 8 {
		return errors.New("invalid monitor event date")
	}
	switch event.Outcome {
	case MonitorFired, MonitorQuiet, MonitorSuppressed, MonitorSourceDown:
	default:
		return errors.New("invalid monitor outcome")
	}
	if len(event.Detail) > 512 {
		return errors.New("monitor detail too long")
	}
	if event.CreatedAt.IsZero() {
		return errors.New("monitor event timestamp required")
	}
	return nil
}

// FileMonitorStore persists monitor rules and their daily outcomes under a
// private directory. Corrupt records fail closed.
type FileMonitorStore struct {
	dir string
	mu  sync.Mutex
}

func NewFileMonitorStore(dir string) (*FileMonitorStore, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("monitor directory required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("monitor directory must be private")
	}
	return &FileMonitorStore{dir: dir}, nil
}

func (s *FileMonitorStore) rulePath(id string) string {
	return filepath.Join(s.dir, "rule-"+id+".json")
}

// SaveRule creates or updates one rule. Kind is immutable like schedule tasks.
func (s *FileMonitorStore) SaveRule(rule MonitorRule) error {
	if s == nil {
		return errors.New("nil monitor store")
	}
	if err := validateMonitorRule(rule); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, err := s.readRule(rule.ID); err == nil && old.Kind != rule.Kind {
		return errors.New("monitor kind is immutable")
	}
	return writeJSONFile(s.rulePath(rule.ID), rule)
}

func (s *FileMonitorStore) readRule(id string) (MonitorRule, error) {
	if !monitorIDPattern.MatchString(id) {
		return MonitorRule{}, errors.New("invalid monitor rule id")
	}
	raw, err := os.ReadFile(s.rulePath(id))
	if err != nil {
		return MonitorRule{}, err
	}
	if len(raw) > (16 << 10) {
		return MonitorRule{}, errors.New("monitor rule too large")
	}
	var rule MonitorRule
	if err := json.Unmarshal(raw, &rule); err != nil {
		return MonitorRule{}, err
	}
	if err := validateMonitorRule(rule); err != nil {
		return MonitorRule{}, err
	}
	if rule.ID != id {
		return MonitorRule{}, fmt.Errorf("monitor rule id mismatch %s", id)
	}
	return rule, nil
}

func (s *FileMonitorStore) GetRule(id string) (MonitorRule, error) {
	if s == nil {
		return MonitorRule{}, errors.New("nil monitor store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readRule(id)
}

func (s *FileMonitorStore) ListRules() ([]MonitorRule, error) {
	if s == nil {
		return nil, errors.New("nil monitor store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	rules := make([]MonitorRule, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "rule-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(entry.Name(), "rule-"), ".json")
		rule, err := s.readRule(id)
		if err != nil {
			return nil, fmt.Errorf("monitor rule %s: %w", id, err)
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func (s *FileMonitorStore) DeleteRule(id string) error {
	if s == nil {
		return errors.New("nil monitor store")
	}
	if !monitorIDPattern.MatchString(id) {
		return errors.New("invalid monitor rule id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.rulePath(id)); err != nil {
		return err
	}
	return nil
}

func (s *FileMonitorStore) eventPath(ruleID, tradeDate string) string {
	return filepath.Join(s.dir, "event-"+ruleID+"-"+tradeDate+".json")
}

// SaveEvent durably records one outcome. The (rule, date) pair is enforced
// by exclusive file creation: the first outcome of the day wins.
func (s *FileMonitorStore) SaveEvent(event MonitorEvent) error {
	if s == nil {
		return errors.New("nil monitor store")
	}
	if err := validateMonitorEvent(event); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := os.OpenFile(s.eventPath(event.RuleID, event.TradeDate), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if os.IsExist(err) {
			return ErrMonitorEventExists
		}
		return err
	}
	raw, err := json.Marshal(event)
	if err == nil {
		_, err = file.Write(raw)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		_ = os.Remove(s.eventPath(event.RuleID, event.TradeDate))
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return nil
}

// ErrMonitorEventExists marks the once-per-(rule, day) dedup boundary.
var ErrMonitorEventExists = errors.New("monitor outcome already recorded for this day")

func (s *FileMonitorStore) GetEvent(ruleID, tradeDate string) (MonitorEvent, error) {
	if s == nil {
		return MonitorEvent{}, errors.New("nil monitor store")
	}
	if !monitorIDPattern.MatchString(ruleID) {
		return MonitorEvent{}, errors.New("invalid monitor rule id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readEvent(ruleID, tradeDate)
}

func (s *FileMonitorStore) readEvent(ruleID, tradeDate string) (MonitorEvent, error) {
	raw, err := os.ReadFile(s.eventPath(ruleID, tradeDate))
	if err != nil {
		return MonitorEvent{}, err
	}
	if len(raw) > (8 << 10) {
		return MonitorEvent{}, errors.New("monitor event too large")
	}
	var event MonitorEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return MonitorEvent{}, err
	}
	if err := validateMonitorEvent(event); err != nil {
		return MonitorEvent{}, err
	}
	if event.RuleID != ruleID || event.TradeDate != tradeDate {
		return MonitorEvent{}, errors.New("monitor event identity mismatch")
	}
	return event, nil
}

// ListEvents returns outcomes for one rule across days, newest first.
func (s *FileMonitorStore) ListEvents(ruleID string) ([]MonitorEvent, error) {
	if s == nil {
		return nil, errors.New("nil monitor store")
	}
	if !monitorIDPattern.MatchString(ruleID) {
		return nil, errors.New("invalid monitor rule id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	events := make([]MonitorEvent, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "event-"+ruleID+"-") {
			continue
		}
		datePart := strings.TrimSuffix(strings.TrimPrefix(entry.Name(), "event-"+ruleID+"-"), ".json")
		event, err := s.readEvent(ruleID, datePart)
		if err != nil {
			return nil, fmt.Errorf("monitor event %s: %w", entry.Name(), err)
		}
		events = append(events, event)
	}
	for i := 1; i < len(events); i++ {
		for j := i; j > 0 && events[j].TradeDate > events[j-1].TradeDate; j-- {
			events[j], events[j-1] = events[j-1], events[j]
		}
	}
	return events, nil
}

// LatestEvent returns the most recent outcome for one rule.
func (s *FileMonitorStore) LatestEvent(ruleID string) (MonitorEvent, error) {
	events, err := s.ListEvents(ruleID)
	if err != nil {
		return MonitorEvent{}, err
	}
	if len(events) == 0 {
		return MonitorEvent{}, os.ErrNotExist
	}
	return events[0], nil
}

// formatThreshold renders one threshold for event details.
func formatThreshold(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// CreateRule assigns nothing itself; the HTTP layer owns ID/timestamps.
func (s *FileMonitorStore) CreateRule(rule MonitorRule) (MonitorRule, error) {
	if err := s.SaveRule(rule); err != nil {
		return MonitorRule{}, err
	}
	return rule, nil
}

// UpdateRule replaces one stored rule (kind stays immutable).
func (s *FileMonitorStore) UpdateRule(rule MonitorRule) error { return s.SaveRule(rule) }
