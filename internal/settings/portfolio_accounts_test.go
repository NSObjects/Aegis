package settings

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Aegis/internal/portfolio"
)

func portfolioImportFixture(now time.Time) AccountImport {
	asOf := now.Add(-time.Hour).Format(time.RFC3339)
	limit, budget := int64(3000), int64(0)
	return AccountImport{
		Snapshot: portfolio.ResearchAccountSnapshot{
			AccountID: "fixture-account", Version: "snapshot-1",
			Source: &portfolio.ResearchSource{Kind: portfolio.ResearchSourceOperator, Reference: "fixture-statement"},
			Book: portfolio.Context{State: portfolio.StateEmpty, AsOf: asOf, Valuation: &portfolio.Valuation{
				Scope: portfolio.ValuationScopeCashAndPositions, Currency: portfolio.CurrencyCNY,
				TotalValueFen: 500_000, CashFen: 500_000, PositionValues: []portfolio.PositionValue{},
			}},
		},
		Policy: &portfolio.ResearchRiskPolicy{
			Version: "policy-1", Source: portfolio.ResearchSource{Kind: portfolio.ResearchSourceOperator, Reference: "fixture-risk-policy"},
			Limits: portfolio.Limits{PolicyID: "fixture-policy", AsOf: asOf, Scope: portfolio.ValuationScopeCashAndPositions,
				Currency: portfolio.CurrencyCNY, Rules: []portfolio.LimitRule{{Kind: portfolio.LimitMaxInvestedWeight, LimitBps: &limit}}},
			LossBudgetFen: &budget,
		},
	}
}

func TestPortfolioAccountStoreEncryptsAndKeepsIndependentVersions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "versions.enc")
	key := strings.Repeat("1a", 32)
	store, err := NewPortfolioAccountStore(path, key, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 19, 8, 0, 0, 0, time.UTC)
	input := portfolioImportFixture(now)
	view, created, err := store.Import(input, now)
	if err != nil || !created || view.Exposure == nil || view.Exposure.CashFen != 500_000 ||
		view.LossBudgetFen == nil || *view.LossBudgetFen != 0 || !view.AccountSource.AvailableAt.Equal(now) {
		t.Fatalf("unexpected import: %+v, %v", view, err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || bytes.Contains(raw, []byte("fixture-account")) || bytes.Contains(raw, []byte("500000")) {
		t.Fatalf("plaintext account data on disk or unreadable file: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("expected private archive: %v, %v", info, err)
	}
	store, err = NewPortfolioAccountStore(path, key, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	retry, created, err := store.Import(input, now.Add(time.Minute))
	if err != nil || created || !retry.AccountSource.AvailableAt.Equal(now) {
		t.Fatalf("retry changed observation availability: %+v, %v", retry, err)
	}
	input.Snapshot.Version = "snapshot-2"
	*input.Policy.LossBudgetFen = 100
	if _, _, err := store.Import(input, now.Add(time.Minute)); !errors.Is(err, ErrAccountVersionConflict) {
		t.Fatalf("changed policy reused an immutable version: %v", err)
	}
	if _, err := store.Get(input.Snapshot.AccountID, "snapshot-2", "", now.Add(time.Minute)); !errors.Is(err, ErrAccountVersionMissing) {
		t.Fatalf("failed import left partial snapshot: %v", err)
	}
	input.Policy.Version = "policy-2"
	updated, created, err := store.Import(input, now.Add(time.Minute))
	if err != nil || !created || updated.PolicyVersion != "policy-2" {
		t.Fatalf("valid independent versions rejected: %+v, %v", updated, err)
	}
	if _, err := store.Get(input.Snapshot.AccountID, "snapshot-1", "policy-2", now); !errors.Is(err, ErrAccountImportInvalid) {
		t.Fatalf("future policy leaked into historical query: %v", err)
	}
	first, err := store.Get(input.Snapshot.AccountID, "snapshot-1", "policy-1", now)
	if err != nil || first.LossBudgetFen == nil || *first.LossBudgetFen != 0 {
		t.Fatalf("immutable policy changed: %+v, %v", first, err)
	}

	foreign, err := NewPortfolioAccountStore(path, strings.Repeat("2b", 32), 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := foreign.Get(input.Snapshot.AccountID, "snapshot-1", "policy-1", now); err == nil {
		t.Fatal("wrong encryption key recovered account data")
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 1
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(input.Snapshot.AccountID, "snapshot-1", "policy-1", now); err == nil {
		t.Fatal("corrupted ciphertext recovered account data")
	}
}

func TestPortfolioAccountStoreRejectsUnreconciledAndBackdatedInputs(t *testing.T) {
	now := time.Date(2026, 11, 19, 8, 0, 0, 0, time.UTC)
	store, err := NewPortfolioAccountStore(filepath.Join(t.TempDir(), "private", "versions.enc"), strings.Repeat("ab", 32), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*AccountImport){
		func(in *AccountImport) { in.Snapshot.Source.AvailableAt = now.Add(-time.Second) },
		func(in *AccountImport) { in.Snapshot.Source.Kind = portfolio.ResearchSourceProvider },
		func(in *AccountImport) { in.Snapshot.Book.Valuation.TotalValueFen-- },
		func(in *AccountImport) { in.Snapshot.Book.AsOf = now.Add(-2 * time.Hour).Format(time.RFC3339) },
	} {
		input := portfolioImportFixture(now)
		change(&input)
		if _, _, err := store.Import(input, now); !errors.Is(err, ErrAccountImportInvalid) {
			t.Fatalf("invalid account data accepted: %v", err)
		}
	}
}
