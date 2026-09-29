package event

import (
	"strings"
	"testing"
)

func TestPrepareAcquisitionStepTaskPreservesTargetAcrossBroadCandidate(
	t *testing.T,
) {
	prepared, err := prepareAcquisitionStepTask(
		ResearchTask{
			ID:     "root",
			Symbol: "600519.SH",
		},
		ResearchTask{
			InformationNeed: NeedBreakingNews,
		},
	)
	if err != nil {
		t.Fatalf(
			"prepareAcquisitionStepTask() error = %v",
			err,
		)
	}
	if prepared.Symbol != "" {
		t.Fatalf(
			"candidate Symbol = %q, want empty",
			prepared.Symbol,
		)
	}
	if prepared.TargetSymbol != "600519.SH" {
		t.Fatalf(
			"candidate TargetSymbol = %q, want 600519.SH",
			prepared.TargetSymbol,
		)
	}
}

func TestPrepareAcquisitionStepTaskRejectsConflictingTarget(
	t *testing.T,
) {
	_, err := prepareAcquisitionStepTask(
		ResearchTask{
			ID:           "root",
			Symbol:       "600519.SH",
			TargetSymbol: "600519.SH",
		},
		ResearchTask{
			TargetSymbol: "000001.SZ",
		},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"must match root target symbol",
		) {
		t.Fatalf(
			"error = %v, want target mismatch",
			err,
		)
	}
}
