package application

import (
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

// WatchlistMaxItems bounds the watchlist so a runaway client cannot grow an
// unbounded on-disk set.
const WatchlistMaxItems = 50

var watchSymbolPattern = regexp.MustCompile(`^[0-9]{6}\.(SH|SZ)$`)

// Sentinel errors for HTTP status mapping; they carry no operator content.
var (
 ErrWatchlistDuplicate = errors.New("watchlist symbol already present")
 ErrWatchlistFull = errors.New("watchlist capacity reached")
)

// WatchItem is one operator-curated observation entry. It stores only the
// operator's own reason and watch conditions; it is never a research verdict,
// a market snapshot, or an executable order.
type WatchItem struct {
 Symbol string `json:"symbol"`
 Reason string `json:"reason"`
 Conditions string `json:"conditions"`
 CreatedAt time.Time `json:"created_at"`
 UpdatedAt time.Time `json:"updated_at"`
}

func ValidWatchSymbol(s string) bool { return watchSymbolPattern.MatchString(s) }

func validateWatchItem(item WatchItem) error {
 if !ValidWatchSymbol(item.Symbol) { return errors.New("invalid watchlist symbol") }
 if item.Reason == "" || len(item.Reason) > 1024 { return errors.New("watch reason must be 1..1024 bytes") }
 if len(item.Conditions) > 1024 { return errors.New("watch conditions too long") }
 if item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() || item.UpdatedAt.Before(item.CreatedAt) { return errors.New("invalid watch timestamps") }
 return nil
}

// FileWatchlistStore persists one JSON file per symbol under a private
// directory. Deleting a missing item reports os.ErrNotExist; corrupt files
// fail closed instead of being silently skipped.
type FileWatchlistStore struct { dir string; mu sync.Mutex }

func NewFileWatchlistStore(dir string) (*FileWatchlistStore, error) {
 if strings.TrimSpace(dir) == "" { return nil, errors.New("watchlist directory required") }
 if err := os.MkdirAll(dir, 0700); err != nil { return nil, err }
 info, err := os.Stat(dir); if err != nil { return nil, err }
 if !info.IsDir() || info.Mode().Perm()&0077 != 0 { return nil, errors.New("watchlist directory must be private") }
 return &FileWatchlistStore{dir: dir}, nil
}

func (s *FileWatchlistStore) path(symbol string) string { return filepath.Join(s.dir, symbol+".json") }

// Add durably creates one item. The store owns timestamps. A duplicate symbol
// returns ErrWatchlistDuplicate and a full watchlist ErrWatchlistFull; on any
// write error the half-created file is removed again.
func (s *FileWatchlistStore) Add(symbol, reason, conditions string) (WatchItem, error) {
 if s == nil { return WatchItem{}, errors.New("nil watchlist store") }
 now := time.Now().UTC()
 item := WatchItem{Symbol: symbol, Reason: reason, Conditions: conditions, CreatedAt: now, UpdatedAt: now}
 if err := validateWatchItem(item); err != nil { return WatchItem{}, err }
 s.mu.Lock(); defer s.mu.Unlock()
 // Reuse the full parse-and-validate listing: an unreadable watchlist must
 // refuse new writes too, not just refuse reads.
 persisted, err := s.listLocked()
 if err != nil { return WatchItem{}, err }
 if len(persisted) >= WatchlistMaxItems { return WatchItem{}, ErrWatchlistFull }
 file, err := os.OpenFile(s.path(symbol), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
 if err != nil {
  if os.IsExist(err) { return WatchItem{}, ErrWatchlistDuplicate }
  return WatchItem{}, err
 }
 raw, err := json.Marshal(item)
 if err == nil { _, err = file.Write(raw) }
 if err == nil { err = file.Sync() }
 closeErr := file.Close()
 if err != nil { _ = os.Remove(s.path(symbol)); return WatchItem{}, err }
 if closeErr != nil { return WatchItem{}, closeErr }
 return item, nil
}

// Remove deletes one item. Missing items report os.ErrNotExist.
func (s *FileWatchlistStore) Remove(symbol string) error {
 if s == nil { return errors.New("nil watchlist store") }
 if !ValidWatchSymbol(symbol) { return errors.New("invalid watchlist symbol") }
 s.mu.Lock(); defer s.mu.Unlock()
 if err := os.Remove(s.path(symbol)); err != nil { return err }
 return nil
}

// List returns all items sorted by symbol. Any unreadable or invalid file
// fails the whole listing: never present a partial watchlist as complete.
func (s *FileWatchlistStore) List() ([]WatchItem, error) {
 if s == nil { return nil, errors.New("nil watchlist store") }
 s.mu.Lock(); defer s.mu.Unlock()
 return s.listLocked()
}

// listLocked parses and validates every persisted item; callers hold s.mu.
func (s *FileWatchlistStore) listLocked() ([]WatchItem, error) {
 entries, err := os.ReadDir(s.dir)
 if err != nil { return nil, err }
 items := make([]WatchItem, 0, len(entries))
 for _, entry := range entries {
  if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") { continue }
  symbol := strings.TrimSuffix(entry.Name(), ".json")
  if !ValidWatchSymbol(symbol) { return nil, fmt.Errorf("invalid watchlist filename %q", entry.Name()) }
  raw, err := os.ReadFile(s.path(symbol))
  if err != nil { return nil, err }
  if len(raw) > (8 << 10) { return nil, errors.New("watch item too large") }
  var item WatchItem
  if err := json.Unmarshal(raw, &item); err != nil { return nil, err }
  if err := validateWatchItem(item); err != nil { return nil, err }
  if item.Symbol != symbol { return nil, fmt.Errorf("watch item symbol mismatch %q", symbol) }
  items = append(items, item)
 }
 sort.Slice(items, func(i, j int) bool { return items[i].Symbol < items[j].Symbol })
 return items, nil
}
