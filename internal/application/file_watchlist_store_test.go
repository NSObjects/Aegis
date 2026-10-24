package application

import (
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestFileWatchlistStoreAddListRemoveRoundTrip(t *testing.T) {
 dir := filepath.Join(t.TempDir(), "watchlist")
 store, err := NewFileWatchlistStore(dir)
 if err != nil { t.Fatalf("open store: %v", err) }
 items, err := store.List()
 if err != nil { t.Fatalf("empty list: %v", err) }
 if len(items) != 0 { t.Fatalf("fresh watchlist not empty: %+v", items) }
 first, err := store.Add("600519.SH", "回调接近前低", "跌破 MA20 时提醒")
 if err != nil { t.Fatalf("add: %v", err) }
 if first.CreatedAt.IsZero() || !first.UpdatedAt.Equal(first.CreatedAt) { t.Fatalf("store must own timestamps: %+v", first) }
 if _, err := store.Add("000001.SZ", "等待量能收缩", ""); err != nil { t.Fatalf("add second: %v", err) }
 items, err = store.List()
 if err != nil { t.Fatalf("list: %v", err) }
 if len(items) != 2 || items[0].Symbol != "000001.SZ" || items[1].Symbol != "600519.SH" {
  t.Fatalf("items not sorted by symbol: %+v", items)
 }
 if err := store.Remove("600519.SH"); err != nil { t.Fatalf("remove: %v", err) }
 items, err = store.List()
 if err != nil || len(items) != 1 || items[0].Symbol != "000001.SZ" { t.Fatalf("after remove: %+v err=%v", items, err) }
 if err := store.Remove("600519.SH"); !errors.Is(err, os.ErrNotExist) { t.Fatalf("missing remove err=%v", err) }
}

func TestFileWatchlistSurvivesRestart(t *testing.T) {
 dir := filepath.Join(t.TempDir(), "watchlist")
 store, err := NewFileWatchlistStore(dir)
 if err != nil { t.Fatal(err) }
 if _, err := store.Add("600519.SH", "reason", "condition"); err != nil { t.Fatal(err) }
 // A new process is a new store instance over the same directory.
 reopened, err := NewFileWatchlistStore(dir)
 if err != nil { t.Fatal(err) }
 items, err := reopened.List()
 if err != nil { t.Fatalf("reopen list: %v", err) }
 if len(items) != 1 || items[0].Symbol != "600519.SH" || items[0].Reason != "reason" || items[0].Conditions != "condition" {
  t.Fatalf("watchlist not retained across restart: %+v", items)
 }
}

func TestFileWatchlistAddValidationAndDuplicates(t *testing.T) {
 store, err := NewFileWatchlistStore(filepath.Join(t.TempDir(), "watchlist"))
 if err != nil { t.Fatal(err) }
 if _, err := store.Add("600519.SH", "reason", "cond"); err != nil { t.Fatal(err) }
 if _, err := store.Add("600519.SH", "another", ""); !errors.Is(err, ErrWatchlistDuplicate) { t.Fatalf("duplicate err=%v", err) }
 for _, tc := range []struct{ name, symbol, reason, conditions string }{
  {"bad symbol", "600519", "reason", ""},
  {"empty reason", "000001.SZ", "", ""},
  {"oversized reason", "000001.SZ", strings.Repeat("r", 1025), ""},
  {"oversized conditions", "000001.SZ", "reason", strings.Repeat("c", 1025)},
 } {
  if _, err := store.Add(tc.symbol, tc.reason, tc.conditions); err == nil {
   t.Errorf("%s accepted", tc.name)
  }
 }
 items, err := store.List()
 if err != nil || len(items) != 1 { t.Fatalf("rejected adds must not persist: %+v err=%v", items, err) }
}

func TestFileWatchlistCapacityBounded(t *testing.T) {
 store, err := NewFileWatchlistStore(filepath.Join(t.TempDir(), "watchlist"))
 if err != nil { t.Fatal(err) }
 for i := 0; i < WatchlistMaxItems; i++ {
  if _, err := store.Add(fmt.Sprintf("%06d.SZ", i), "reason", ""); err != nil { t.Fatalf("fill %d: %v", i, err) }
 }
 if _, err := store.Add("688981.SH", "overflow", ""); !errors.Is(err, ErrWatchlistFull) { t.Fatalf("capacity err=%v", err) }
 items, err := store.List()
 if err != nil || len(items) != WatchlistMaxItems { t.Fatalf("count=%d err=%v", len(items), err) }
}

func TestFileWatchlistStoreRejectsSharedDirectory(t *testing.T) {
 dir := t.TempDir()
 if err := os.Chmod(dir, 0750); err != nil { t.Fatal(err) }
 if _, err := NewFileWatchlistStore(dir); err == nil { t.Fatal("shared-permission directory accepted") }
 if err := os.Chmod(dir, 0700); err != nil { t.Fatal(err) }
 if _, err := NewFileWatchlistStore(dir); err != nil { t.Fatalf("private directory rejected: %v", err) }
}

func TestFileWatchlistListFailsClosedOnCorruption(t *testing.T) {
 dir := filepath.Join(t.TempDir(), "watchlist")
 store, err := NewFileWatchlistStore(dir)
 if err != nil { t.Fatal(err) }
 if _, err := store.Add("600519.SH", "reason", ""); err != nil { t.Fatal(err) }
 if err := os.WriteFile(filepath.Join(dir, "000001.SZ.json"), []byte("{not-json"), 0600); err != nil { t.Fatal(err) }
 if _, err := store.List(); err == nil { t.Fatal("corrupt item did not fail the listing") }
 if _, err := store.Add("300750.SZ", "reason", ""); err == nil { t.Fatal("adds must stop when the store cannot be trusted") }
 // A tampered valid-JSON item must also fail validation rather than serve.
 tampered := `{"symbol":"000001.SZ","reason":"","conditions":"","created_at":"2026-10-24T00:00:00Z","updated_at":"2026-10-24T00:00:00Z"}`
 if err := os.WriteFile(filepath.Join(dir, "000001.SZ.json"), []byte(tampered), 0600); err != nil { t.Fatal(err) }
 if _, err := store.List(); err == nil { t.Fatal("invalid item accepted") }
}
