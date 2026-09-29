package event

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLiveCninfoAnnouncementRetriever(t *testing.T) {
	if os.Getenv("AEGIS_LIVE_CNINFO") != "1" {
		t.Skip("set AEGIS_LIVE_CNINFO=1 to run live Cninfo integration test")
	}

	location := cninfoShanghaiLocation
	asOf := time.Date(
		2026, 9, 24, 23, 59, 59, 0, location,
	)
	task := ResearchTask{
		ID:              "live-cninfo-600519",
		Symbol:          "600519.SH",
		InformationNeed: NeedOfficialDisclosure,
		AsOf:            asOf,
		TimeRange: TimeRange{
			Start: time.Date(
				2026, 1, 1, 0, 0, 0, 0, location,
			),
			End: asOf,
		},
		Requirements: ResearchRequirements{
			PITSafe:               true,
			RequiredSourceTypes:   []SourceType{SourceOfficial},
			MinDocuments:          1,
			MinIndependentSources: 1,
			RequirePrimarySource:  true,
		},
	}

	output, err := (TaskRunner{}).Run(
		context.Background(),
		task,
		CninfoAnnouncementRetriever{
			PageSize: 30,
			MaxPages: 2,
		},
	)
	if err != nil {
		t.Fatalf("live Run() error = %v", err)
	}

	if output.Attempt.Status != AcquisitionSucceeded {
		t.Fatalf(
			"attempt status = %q; error=%q",
			output.Attempt.Status,
			output.Attempt.ErrorMessage,
		)
	}
	if len(output.RawDocuments) == 0 {
		t.Fatal("live Cninfo returned no raw documents for 600519.SH")
	}
	if len(output.Canonicalization.Documents) == 0 {
		t.Fatal("live Cninfo returned no PIT-safe canonical documents for 600519.SH")
	}
	if output.Result.Status != TaskSatisfied {
		t.Fatalf(
			"task status = %q; gaps=%v",
			output.Result.Status,
			output.Result.Gaps,
		)
	}

	for _, document := range output.RawDocuments {
		if document.SourceType != SourceOfficial {
			t.Fatalf(
				"source type = %q, want %q",
				document.SourceType,
				SourceOfficial,
			)
		}
		if !document.IsPrimary {
			t.Fatal("live Cninfo document should be primary evidence")
		}
		if document.Publisher != cninfoPublisher {
			t.Fatalf(
				"publisher = %q, want %q",
				document.Publisher,
				cninfoPublisher,
			)
		}
	}
}
