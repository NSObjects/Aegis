package semanticreview

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func fixtureCurationBatch() *CurationBatch {
	base, _ := evaluationFixture(ProvenanceSynthetic)
	mkSample := func(id string, split DatasetSplit, group string, left, right Verdict) CurationSample {
		tasks := base.Samples[0].Tasks // fictional research tasks, not real evidence
		copyTasks := *tasks
		copyTasks.Items = append([]Task(nil), tasks.Items...)
		hash := sha256.Sum256([]byte("synthetic-source-" + id))
		first := GoldLabel{InsightIndex: 0, ArgumentID: "arg-a", ExpectedVerdict: left}
		second := GoldLabel{InsightIndex: 0, ArgumentID: "arg-a", ExpectedVerdict: right}
		return CurationSample{
			ID: id, Split: split, LeakageGroup: group,
			Source: CurationSource{
				EvidenceRef: "fictional-fixture/" + id,
				EvidenceSHA256: hex.EncodeToString(hash[:]),
				EvidenceAsOf: "2026-09-29T10:00:00Z",
				CapturedAt: "2026-09-29T11:00:00Z",
				RequestedAsOf: "2026-09-30T10:00:00Z",
			},
			Tasks: &copyTasks,
			Annotations: []AnnotationAttestation{
				{AnnotatorID: "fictional-human-a", BlindToModel: true, BlindToPeer: true, Labels: []GoldLabel{first}},
				{AnnotatorID: "fictional-human-b", BlindToModel: true, BlindToPeer: true, Labels: []GoldLabel{second}},
			},
		}
	}
	development := mkSample("dev-fixture", SplitDevelopment, "dev-group", VerdictConsistent, VerdictConsistent)
	holdout := mkSample("holdout-fixture", SplitHoldout, "holdout-group", VerdictConsistent, VerdictContradictory)
	holdout.Adjudications = []Adjudication{{
		AdjudicatorID: "fictional-adjudicator",
		Label: GoldLabel{InsightIndex: 0, ArgumentID: "arg-a", ExpectedVerdict: VerdictUncertain},
		Rationale: "fictional sample lacks sufficient semantic support",
	}}
	return &CurationBatch{Version: "fixture-curation-v1", Samples: []CurationSample{development, holdout}}
}

func TestFreezeCurationProducesSealedAndSeparateDatasets(t *testing.T) {
	source := fixtureCurationBatch()
	before, err := json.Marshal(source)
	if err != nil { t.Fatal(err) }
	first, err := FreezeCuration(source)
	if err != nil { t.Fatal(err) }
	second, err := FreezeCuration(source)
	if err != nil { t.Fatal(err) }
	if !reflect.DeepEqual(first, second) {
		t.Fatal("identical curation must produce byte-identical snapshots and manifest")
	}
	if first.Manifest.DevelopmentSamples != 1 ||
		first.Manifest.HoldoutSamples != 1 ||
		first.Manifest.AdjudicatedItems != 1 {
		t.Fatalf("invalid manifest counts: %+v", first.Manifest)
	}
	if err := VerifyFrozenCuration(first); err != nil { t.Fatal(err) }
	for _, split := range []DatasetSplit{SplitDevelopment, SplitHoldout} {
		dataset, err := ReadFrozenDataset(first, split)
		if err != nil { t.Fatal(err) }
		if len(dataset.Samples) != 1 || dataset.Provenance != ProvenanceHumanReviewed ||
			len(dataset.Samples[0].AnnotatorIDs) != 2 {
			t.Fatalf("invalid %s split: %+v", split, dataset)
		}
		if split == SplitDevelopment && dataset.Samples[0].Gold[0].ExpectedVerdict != VerdictConsistent {
			t.Fatalf("agreed verdict changed: %+v", dataset)
		}
		if split == SplitHoldout && dataset.Samples[0].Gold[0].ExpectedVerdict != VerdictUncertain {
			t.Fatalf("adjudicated verdict not honored: %+v", dataset)
		}
		dataset.Samples[0].Gold[0].ExpectedVerdict = VerdictContradictory
	}
	after, err := json.Marshal(source)
	if err != nil { t.Fatal(err) }
	if string(before) != string(after) { t.Fatal("FreezeCuration mutated original input") }
	if err := VerifyFrozenCuration(first); err != nil { t.Fatalf("caller modifications affected frozen output: %v", err) }

	var roundtrip FrozenCuration
	serialized, err := json.Marshal(first)
	if err != nil { t.Fatal(err) }
	if err := json.Unmarshal(serialized, &roundtrip); err != nil { t.Fatal(err) }
	if err := VerifyFrozenCuration(&roundtrip); err != nil { t.Fatal(err) }
}

func TestFreezeCurationRejectsUntrustedAnnotationsAndPITLeakage(t *testing.T) {
	tests := []struct {
		name string
		change func(*CurationBatch)
		want string
	}{
		{"missing second annotator", func(s *CurationBatch) { s.Samples[0].Annotations = s.Samples[0].Annotations[:1] }, "two independent"},
		{"same annotator twice", func(s *CurationBatch) { s.Samples[0].Annotations[1].AnnotatorID = "fictional-human-a" }, "distinct"},
		{"model not blind", func(s *CurationBatch) { s.Samples[0].Annotations[0].BlindToModel = false }, "model-blind"},
		{"peer not blind", func(s *CurationBatch) { s.Samples[0].Annotations[1].BlindToPeer = false }, "peer-blind"},
		{"missing task label", func(s *CurationBatch) { s.Samples[0].Annotations[0].Labels = nil }, "cover"},
		{"forged task identity", func(s *CurationBatch) { s.Samples[0].Annotations[0].Labels[0].ArgumentID = "ghost" }, "identity"},
		{"invalid verdict", func(s *CurationBatch) { s.Samples[0].Annotations[0].Labels[0].ExpectedVerdict = "IMPOSSIBLE" }, "verdict"},
		{"unadjudicated", func(s *CurationBatch) { s.Samples[1].Adjudications = nil }, "lacks adjudication"},
		{"adjudicator is annotator", func(s *CurationBatch) { s.Samples[1].Adjudications[0].AdjudicatorID = "fictional-human-a" }, "adjudicator"},
		{"adjudicator missing rationale", func(s *CurationBatch) { s.Samples[1].Adjudications[0].Rationale = "" }, "rationale"},
		{"adjudicator for wrong task", func(s *CurationBatch) { s.Samples[1].Adjudications[0].Label.ArgumentID = "ghost" }, "adjudication"},
		{"unneeded adjudication", func(s *CurationBatch) { s.Samples[0].Adjudications = append(s.Samples[0].Adjudications, s.Samples[1].Adjudications[0]) }, "unnecessary"},
		{"sample duplicated", func(s *CurationBatch) { s.Samples[1].ID = s.Samples[0].ID }, "duplicate"},
		{"same research event in holdout", func(s *CurationBatch) { s.Samples[1].LeakageGroup = s.Samples[0].LeakageGroup }, "leakage_group"},
		{"same source snapshot in holdout", func(s *CurationBatch) { s.Samples[1].Source.EvidenceSHA256 = s.Samples[0].Source.EvidenceSHA256 }, "source snapshot"},
		{"malformed snapshot hash", func(s *CurationBatch) { s.Samples[0].Source.EvidenceSHA256 = "invented" }, "evidence_sha256"},
		{"no evidence ref", func(s *CurationBatch) { s.Samples[0].Source.EvidenceRef = "" }, "evidence_ref"},
		{"captured after research cutoff", func(s *CurationBatch) { s.Samples[0].Source.CapturedAt = "2026-10-01T10:00:00Z" }, "point-in-time"},
		{"evidence later than capture", func(s *CurationBatch) { s.Samples[0].Source.EvidenceAsOf = "2026-09-30T09:00:00Z" }, "point-in-time"},
		{"nonparseable timestamp", func(s *CurationBatch) { s.Samples[0].Source.RequestedAsOf = "today" }, "requested_as_of"},
		{"missing holdout", func(s *CurationBatch) { s.Samples[1].Split = SplitDevelopment }, "nonempty"},
		{"invalid split", func(s *CurationBatch) { s.Samples[0].Split = "TEST" }, "split"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := fixtureCurationBatch()
			tc.change(input)
			got, err := FreezeCuration(input)
			if got != nil || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("invalid curated data accepted: frozen=%+v err=%v (want %s)", got, err, tc.want)
			}
		})
	}
}

func TestVerifyFrozenCurationRejectsModifiedPayloads(t *testing.T) {
	for _, tc := range []struct {
		name string
		mutate func(*FrozenCuration)
	}{
		{"holdout bytes", func(f *FrozenCuration) {
			f.HoldoutJSON = append([]byte(nil), f.DevelopmentJSON...)
		}},
		{"source bytes", func(f *FrozenCuration) {
			f.CorpusJSON = append([]byte(nil), f.CorpusJSON...)
			f.CorpusJSON[0] = '!'
		}},
		{"falsified manifest", func(f *FrozenCuration) {
			f.Manifest.HoldoutSamples = 1000
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			frozen, err := FreezeCuration(fixtureCurationBatch())
			if err != nil { t.Fatal(err) }
			tc.mutate(frozen)
			if err := VerifyFrozenCuration(frozen); err == nil {
				t.Fatal("corrupted frozen data was accepted")
			}
		})
	}
}
