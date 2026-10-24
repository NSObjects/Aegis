package application

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"Aegis/internal/capability/technical"
)

// TestScanReplayRealTushareIntegration exercises the full reproducibility
// chain against the real Tushare provider: submit a bounded scan, wait for a
// terminal state, then replay the persisted snapshots and require that the
// replayed results equal the persisted ones symbol for symbol. It performs no
// LLM calls and no trading actions. Opt-in via AEGIS_INTEGRATION=1 plus
// TUSHARE_TOKEN, mirroring the other hosted integration tests.
func TestScanReplayRealTushareIntegration(t *testing.T) {
	if os.Getenv("AEGIS_INTEGRATION") != "1" {
		t.Skip("set AEGIS_INTEGRATION=1 to run integration test")
	}
	token := os.Getenv("TUSHARE_TOKEN")
	if token == "" {
		t.Fatal("TUSHARE_TOKEN is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	storeDir := filepath.Join(t.TempDir(), "private-scans")
	store, err := NewFileScanStore(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	researcher, err := technical.NewHTTPResearcher(token)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewScanManager(store, researcher)
	if err != nil {
		t.Fatal(err)
	}
	// Two liquid symbols: one covers a full observable series, the other
	// exercises the same provider path regardless of which verdict it earns.
	submitted, err := manager.Submit(ctx, "20261022", "USER_SUPPLIED", []string{"600519.SH", "000001.SZ"})
	if err != nil {
		t.Fatalf("submit real scan: %v", err)
	}
	deadline := time.Now().Add(5 * time.Minute)
	var final ScanRecord
	for {
		final, err = manager.Get(submitted.ID)
		if err != nil {
			t.Fatal(err)
		}
		if final.State == ScanCompleted || final.State == ScanFailed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("real scan never reached terminal state: %+v", final)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	if final.State != ScanCompleted {
		t.Fatalf("real scan failed: state=%s failure_code=%s", final.State, final.FailureCode)
	}
	if len(final.Results) != 2 {
		t.Fatalf("expected two results: %+v", final)
	}
	replay, err := manager.ReplayScan(final.ID)
	if err != nil {
		t.Fatalf("replay over real scan: %v", err)
	}
	if !replay.Replayable || !replay.Matches {
		t.Fatalf("real scan did not replay identically: %+v", replay)
	}
	if len(replay.Rows) != 2 || len(replay.Unreplayable) != 0 {
		t.Fatalf("expected every symbol replayable: %+v", replay)
	}
	// Replaying again over a freshly reopened store must still match: the
	// digest-anchored snapshots, not in-memory state, are the source of truth.
	reopenedStore, err := NewFileScanStore(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	reopenedManager, err := NewScanManager(reopenedStore, researcher)
	if err != nil {
		t.Fatal(err)
	}
	again, err := reopenedManager.ReplayScan(final.ID)
	if err != nil || !again.Matches || len(again.Rows) != len(replay.Rows) {
		t.Fatalf("second replay diverged: %+v %v", again, err)
	}
	for i, row := range replay.Rows {
		if row != again.Rows[i] {
			t.Fatalf("row %d diverged: %+v vs %+v", i, row, again.Rows[i])
		}
	}
	t.Logf("real scan replay: as_of=%s rows=%d unreplayable=%d", replay.AsOf, len(replay.Rows), len(replay.Unreplayable))
}
