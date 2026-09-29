package event

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeContradictionsDeterministicPair(
	t *testing.T,
) {
	facts := []Fact{
		{ID: "fact_a"},
		{ID: "fact_b"},
	}
	left, err := normalizeContradictions(
		facts,
		[]DetectedContradiction{
			{
				Type:      ContradictionValue,
				FactIDs:   []string{"fact_b", "fact_a"},
				Rationale: " incompatible values ",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	right, err := normalizeContradictions(
		facts,
		[]DetectedContradiction{
			{
				Type:      ContradictionValue,
				FactIDs:   []string{"fact_a", "fact_b"},
				Rationale: "incompatible values",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(left, right) {
		t.Fatalf(
			"normalized contradictions differ:\nleft=%+v\nright=%+v",
			left,
			right,
		)
	}
	if len(left) != 1 ||
		!strings.HasPrefix(left[0].ID, "conflict_") {
		t.Fatalf("contradictions = %+v", left)
	}
}

func TestNormalizeContradictionsCollapsesReversedPair(
	t *testing.T,
) {
	result, err := normalizeContradictions(
		[]Fact{
			{ID: "fact_a"},
			{ID: "fact_b"},
		},
		[]DetectedContradiction{
			{
				Type:      ContradictionStatus,
				FactIDs:   []string{"fact_a", "fact_b"},
				Rationale: "status conflict",
			},
			{
				Type:      ContradictionStatus,
				FactIDs:   []string{"fact_b", "fact_a"},
				Rationale: "duplicate relation",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf(
			"contradictions = %d, want 1",
			len(result),
		)
	}
	if result[0].Rationale !=
		"duplicate relation" {
		t.Fatalf(
			"rationale = %q, want deterministic lexical choice",
			result[0].Rationale,
		)
	}
}

func TestNormalizeContradictionsRejectsMultipleTypesForPair(
	t *testing.T,
) {
	_, err := normalizeContradictions(
		[]Fact{
			{ID: "fact_a"},
			{ID: "fact_b"},
		},
		[]DetectedContradiction{
			{
				Type:      ContradictionValue,
				FactIDs:   []string{"fact_a", "fact_b"},
				Rationale: "value conflict",
			},
			{
				Type:      ContradictionDirect,
				FactIDs:   []string{"fact_b", "fact_a"},
				Rationale: "direct conflict",
			},
		},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"multiple types",
		) {
		t.Fatalf("error = %v", err)
	}
}

func TestNormalizeContradictionsRejectsInvalidRelations(
	t *testing.T,
) {
	facts := []Fact{
		{ID: "fact_a"},
		{ID: "fact_b"},
	}
	tests := []struct {
		name  string
		value DetectedContradiction
		want  string
	}{
		{
			name: "unknown fact",
			value: DetectedContradiction{
				Type:      ContradictionDirect,
				FactIDs:   []string{"fact_a", "unknown"},
				Rationale: "conflict",
			},
			want: "unknown fact",
		},
		{
			name: "same fact",
			value: DetectedContradiction{
				Type:      ContradictionDirect,
				FactIDs:   []string{"fact_a", "fact_a"},
				Rationale: "conflict",
			},
			want: "distinct",
		},
		{
			name: "three facts",
			value: DetectedContradiction{
				Type: ContradictionDirect,
				FactIDs: []string{
					"fact_a", "fact_b", "fact_c",
				},
				Rationale: "conflict",
			},
			want: "exactly two",
		},
		{
			name: "invalid type",
			value: DetectedContradiction{
				Type:      "MAYBE",
				FactIDs:   []string{"fact_a", "fact_b"},
				Rationale: "conflict",
			},
			want: "invalid type",
		},
		{
			name: "empty rationale",
			value: DetectedContradiction{
				Type:      ContradictionDate,
				FactIDs:   []string{"fact_a", "fact_b"},
				Rationale: " ",
			},
			want: "rationale is required",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := normalizeContradictions(
				facts,
				[]DetectedContradiction{
					test.value,
				},
			)
			if err == nil ||
				!strings.Contains(
					err.Error(),
					test.want,
				) {
				t.Fatalf(
					"error = %v, want %q",
					err,
					test.want,
				)
			}
		})
	}
}
