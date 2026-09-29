package event

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLiveCLSHistoricalSnapshotBoundary(t *testing.T) {
	if os.Getenv("AEGIS_LIVE_CLS") != "1" {
		t.Skip("set AEGIS_LIVE_CLS=1 to run live CLS integration test")
	}

	now := time.Now().In(clsShanghaiLocation)
	day := clsStartOfDay(
		now.AddDate(0, 0, -1),
	)
	asOf := clsEndOfDay(day)

	task := ResearchTask{
		ID:              "live-cls-news",
		InformationNeed: NeedBreakingNews,
		AsOf:            asOf,
		TimeRange: TimeRange{
			Start: day,
			End:   asOf,
		},
		Requirements: ResearchRequirements{
			PITSafe:               true,
			RequiredSourceTypes:   []SourceType{SourceIndustryMedia},
			MinDocuments:          1,
			MinIndependentSources: 1,
		},
	}

	output, err := (TaskRunner{}).Run(
		context.Background(),
		task,
		ClsNewsRetriever{
			Category: CLSNewsImportant,
			Limit:    20,
			MaxDays:  1,
		},
	)
	if err != nil {
		t.Fatalf("live CLS Run() error = %v", err)
	}
	if output.Attempt.Status != AcquisitionSucceeded {
		t.Fatalf(
			"attempt status = %q; error=%q",
			output.Attempt.Status,
			output.Attempt.ErrorMessage,
		)
	}
	if len(output.RawDocuments) == 0 {
		t.Fatal("live CLS important feed returned no documents")
	}
	if len(output.Canonicalization.Documents) != 0 {
		t.Fatalf(
			"canonical documents = %d, want 0: current CLS bodies must not be replayed into a historical as_of without stored snapshots",
			len(output.Canonicalization.Documents),
		)
	}
	if len(output.Canonicalization.Rejected) !=
		len(output.RawDocuments) {
		t.Fatalf(
			"rejected documents = %d, raw documents = %d; want every current body rejected for historical replay",
			len(output.Canonicalization.Rejected),
			len(output.RawDocuments),
		)
	}
	for _, rejected := range output.Canonicalization.Rejected {
		if rejected.Reason != RejectionFutureAvailableAt {
			t.Fatalf(
				"rejection reason = %q, want %q",
				rejected.Reason,
				RejectionFutureAvailableAt,
			)
		}
	}
	if output.Result.Status != TaskUnsatisfied {
		t.Fatalf(
			"task status = %q, want %q; gaps=%v",
			output.Result.Status,
			TaskUnsatisfied,
			output.Result.Gaps,
		)
	}

	for _, document := range output.RawDocuments {
		if document.SourceType != SourceIndustryMedia {
			t.Fatalf(
				"source type = %q, want %q",
				document.SourceType,
				SourceIndustryMedia,
			)
		}
		if document.IsPrimary {
			t.Fatal("CLS media evidence must not be primary")
		}
		if document.Publisher != clsPublisher {
			t.Fatalf(
				"publisher = %q, want %q",
				document.Publisher,
				clsPublisher,
			)
		}
	}

	// Company category is a distinct upstream query. It is allowed to be empty
	// for the selected day; this check only verifies that the real endpoint and
	// category signature remain callable.
	_, err = (ClsNewsRetriever{
		Category: CLSNewsCompany,
		Limit:    5,
		MaxDays:  1,
	}).Retrieve(
		context.Background(),
		task,
	)
	if err != nil {
		t.Fatalf(
			"live CLS company category error = %v",
			err,
		)
	}
}
