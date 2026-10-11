package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"Aegis/internal/config"
	"Aegis/internal/decision"
)

func modelFixture(t *testing.T) *RunModelProvenance {
	t.Helper()
	c := &config.Config{
		DefaultModel: "primary",
		Models: map[string]config.ModelConfig{
			"primary": {
				Provider: config.ProviderDeepSeek,
				Model: "deepseek-chat",
				APIKey: "SENSITIVE_API_KEY_MUST_NOT_APPEAR",
				APIKeyEnv: "SENSITIVE_API_ENV_MUST_NOT_APPEAR",
				BaseURL: "https://secret.example?token=SENSITIVE_ENDPOINT_TOKEN",
			},
			"strong": {Provider: config.ProviderDeepSeek, Model: "deepseek-reasoner"},
		},
	}
	meta, err := buildModelProvenance(c, []modelRoleSelection{
		{Role: "market", Profile: "primary"},
		{Role: "trading", Profile: "strong"},
	})
	if err != nil { t.Fatal(err) }
	raw, err := json.Marshal(meta)
	if err != nil { t.Fatal(err) }
	for _, sensitive := range []string{"SENSITIVE_API_KEY", "SENSITIVE_API_ENV", "SENSITIVE_ENDPOINT_TOKEN", "secret.example"} {
		if strings.Contains(string(raw), sensitive) { t.Fatalf("model identity leaked credential or endpoint: %s", raw) }
	}
	if meta.ConfigFingerprint == "" || len(meta.Bindings) != 2 {
		t.Fatalf("invalid configured model metadata: %#v", meta)
	}
	again, err := buildModelProvenance(c, []modelRoleSelection{
		{Role: "market", Profile: "primary"}, {Role: "trading", Profile: "strong"},
	})
	if err != nil || again.ConfigFingerprint != meta.ConfigFingerprint {
		t.Fatal("same bindings changed identity")
	}
	c.Models["strong"] = config.ModelConfig{Provider: config.ProviderDeepSeek, Model: "another-model"}
	changed, err := buildModelProvenance(c, []modelRoleSelection{
		{Role: "market", Profile: "primary"}, {Role: "trading", Profile: "strong"},
	})
	if err != nil || changed.ConfigFingerprint == meta.ConfigFingerprint {
		t.Fatal("different model binding retained the same identity")
	}
	return meta
}

type provenanceExecutor struct {
	meta *RunModelProvenance
	fail bool
}

func (e *provenanceExecutor) Run(ctx context.Context, req ResearchRequest) (*ResearchResponse, error) {
	if err := publishModelProvenance(ctx, e.meta); err != nil { return nil, err }
	if e.fail { return nil, errors.New("simulated model error") }
	return &ResearchResponse{Mode: ModeDecision, Decision: &decision.Decision{
		Symbol: req.Input.Symbol, Action: decision.ActionWatch,
		MarketView: "test fixture", Evidence: []string{"model fixture"},
	}}, nil
}

func TestModelIdentityPersistedAcrossTerminalOutcomesAndRestart(t *testing.T) {
	store := newStore(t)
	meta := modelFixture(t)
	ex := &provenanceExecutor{meta: meta}
	m := newMgr(t, ex, store, 1, 3)
	submitted, err := m.Submit(context.Background(), runFixture("600519.SH"))
	if err != nil { t.Fatal(err) }
	// PENDING metadata must never be attributed to a graph that did not start.
	if submitted.ModelProvenance != nil { t.Fatal("submission fabricated execution identity") }
	done := awaitRun(t, m, submitted.ID, RunCompleted)
	if done.ModelProvenance == nil || done.ModelProvenance.ConfigFingerprint != meta.ConfigFingerprint ||
		len(done.ModelProvenance.Bindings) != 2 {
		t.Fatalf("completed run lost model identity: %#v", done.ModelProvenance)
	}
	ex.fail = true
	second, err := m.Submit(context.Background(), runFixture("000001.SZ"))
	if err != nil { t.Fatal(err) }
	failed := awaitRun(t, m, second.ID, RunFailed)
	if failed.ModelProvenance == nil || failed.ModelProvenance.ConfigFingerprint != meta.ConfigFingerprint ||
		len(failed.Artifact) != 0 {
		t.Fatalf("failed research lost constructed model identity or exposed artifact: %#v", failed)
	}
	m.Close()
	restarted := newMgr(t, &controlledRunExecutor{}, store, 1, 2)
	for _, id := range []string{submitted.ID, second.ID} {
		record, err := restarted.Get(context.Background(), id)
		if err != nil || record.ModelProvenance == nil ||
			record.ModelProvenance.ConfigFingerprint != meta.ConfigFingerprint {
			t.Fatalf("model identity not retained across process restart: %#v %v", record, err)
		}
	}
}

func TestModelIdentityRejectsTamperingAndPreservesLegacyRecords(t *testing.T) {
	store := newStore(t)
	meta := modelFixture(t)
	m := newMgr(t, &provenanceExecutor{meta: meta}, store, 1, 2)
	created, err := m.Submit(context.Background(), runFixture("600519.SH"))
	if err != nil { t.Fatal(err) }
	rec := awaitRun(t, m, created.ID, RunCompleted)
	testCases := []*RunModelProvenance{
		{SchemaVersion: 2, ConfigFingerprint: meta.ConfigFingerprint, Bindings: meta.Bindings},
		{SchemaVersion: 1, ConfigFingerprint: strings.Repeat("0", 64), Bindings: meta.Bindings},
		{SchemaVersion: 1, ConfigFingerprint: meta.ConfigFingerprint, Bindings: append(append([]ModelRoleBinding{}, meta.Bindings...), meta.Bindings[0])},
	}
	for _, invalid := range testCases {
		tampered := rec
		tampered.ModelProvenance = invalid
		if err := store.Save(context.Background(), tampered); err == nil {
			t.Fatalf("accepted invalid provenance: %#v", invalid)
		}
	}
	// A legacy or early-failed task may not have this optional metadata.
	rec.ModelProvenance = nil
	if err := store.Save(context.Background(), rec); err != nil { t.Fatal(err) }
	old, err := store.Get(context.Background(), rec.ID)
	if err != nil || old.ModelProvenance != nil { t.Fatalf("legacy missing-metadata fail: %#v %v", old, err) }
	// Explicitly invalid state must never store a completed graph identity.
	pending := RunRecord{
		ID: strings.Repeat("b", 32), Mode: ModeDecision, Symbol: "600519.SH",
		RequestedAsOf: time.Now().UTC(), CreatedAt: time.Now().UTC(),
		State: RunPending, ModelProvenance: meta,
	}
	if err := store.Save(context.Background(), pending); err == nil {
		t.Fatal("PENDING run persisted a constructed model identity")
	}
}

func TestModelIdentityAbsentWhenExecutorNeverBuildsGraph(t *testing.T) {
	store := newStore(t)
	m := newMgr(t, &controlledRunExecutor{err: errors.New("initialization failed")}, store, 1, 2)
	submitted, err := m.Submit(context.Background(), runFixture("600519.SH"))
	if err != nil { t.Fatal(err) }
	failed := awaitRun(t, m, submitted.ID, RunFailed)
	if failed.ModelProvenance != nil { t.Fatal("initialization failure invented model identity") }
}
