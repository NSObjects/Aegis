package evidenceledger

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestLedgerPreservesExplicitManagerInsightArgumentIDsWithoutInventingLinks(t *testing.T) {
	artifact, state, plan := ledgerFixture()
	plan.Opportunities[0].ArgumentIDs = []string{" bull-1 ", "bull-2"}
	plan.Risks[0].ArgumentIDs = []string{"bear-1"}

	got, err := Build(artifact, state, plan)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if !reflect.DeepEqual(got.Insights[0].ArgumentIDs, []string{"bull-1", "bull-2"}) ||
		!reflect.DeepEqual(got.Insights[1].ArgumentIDs, []string{"bear-1"}) {
		t.Fatalf("explicit links not preserved/normalized: %+v", got.Insights)
	}
	if plan.Opportunities[0].ArgumentIDs[0] != " bull-1 " {
		t.Fatal("Build must not mutate source plan")
	}
	original := append([]string(nil), plan.Opportunities[0].ArgumentIDs...)
	got.Insights[0].ArgumentIDs[0] = "mutated"
	if !reflect.DeepEqual(plan.Opportunities[0].ArgumentIDs, original) {
		t.Fatal("ledger output aliased original insight arguments")
	}

	artifact, state, plan = ledgerFixture()
	legacy, err := Build(artifact, state, plan)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "argument_ids") {
		t.Fatalf("legacy ledger output should not invent argument links: %s", payload)
	}
}

func TestLedgerRejectsUnknownManagerInsightArgumentBeforeProjection(t *testing.T) {
	artifact, state, plan := ledgerFixture()
	plan.Risks[0].ArgumentIDs = []string{"made-up-id"}
	got, err := Build(artifact, state, plan)
	if err == nil || got != nil || !strings.Contains(err.Error(), "does not exist in debate") {
		t.Fatalf("must fail closed on invented Manager provenance: ledger=%+v err=%v", got, err)
	}
}
