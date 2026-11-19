package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestR6EvidenceBindsOriginalBytesAndLeavesReviewPending(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive.json")
	archive := []byte(`{
		"backtest":{
			"policy":{"shares":100,"hold_sessions":1},
			"signals":[
				{"strategy_version":"trend-pullback-v1","cutoff":"2026-11-09T18:00:00+08:00",
				 "calendar_source":"fixture","calendar_known_at":"2026-11-09T08:00:00+08:00",
				 "sessions":["20261109"],"universe_source":"fixture","universe_known_at":"2026-11-09T08:00:00+08:00",
				 "members":[{"symbol":"600519.SH","rows":[]}]},
				{"strategy_version":"trend-pullback-v1","cutoff":"2026-11-11T18:00:00+08:00",
				 "calendar_source":"fixture","calendar_known_at":"2026-11-09T08:00:00+08:00",
				 "sessions":["20261111"],"universe_source":"fixture","universe_known_at":"2026-11-11T08:00:00+08:00",
				 "members":[{"symbol":"600519.SH","rows":[]}]}
			],
			"sessions":[{"date":"20261109"},{"date":"20261110"},{"date":"20261111"},{"date":"20261112"}]
		},
		"folds":[{"train_from":"20261109","train_through":"20261110","test_from":"20261111","test_through":"20261112"}]
	}
	`)
	if err := os.WriteFile(path, archive, 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := r6EvidenceFile(path, &output); err != nil {
		t.Fatal(err)
	}
	var evidence r6Evidence
	if err := json.Unmarshal(output.Bytes(), &evidence); err != nil {
		t.Fatal(err)
	}
	checksum := sha256.Sum256(archive)
	if evidence.ArchiveSHA256 != hex.EncodeToString(checksum[:]) ||
		!evidence.Repeatable || evidence.ArchiveProvenance != "UNVERIFIED" ||
		evidence.ReviewStatus != "PENDING_INDEPENDENT_REVIEW" {
		t.Fatalf("wrong evidence provenance or state: %+v", evidence)
	}
	reportJSON, err := json.Marshal(evidence.Report)
	if err != nil {
		t.Fatal(err)
	}
	checksum = sha256.Sum256(reportJSON)
	if evidence.ReportSHA256 != hex.EncodeToString(checksum[:]) {
		t.Fatalf("report is not bound by its digest: %+v", evidence)
	}
	if len(evidence.OutOfSampleCoverage) != 1 || evidence.OutOfSampleCoverage[0].Insufficient != 1 ||
		evidence.OutOfSampleCoverage[0].ClosedTrades != 0 || evidence.OutOfSampleCoverage[0].Labeled != 0 {
		t.Fatalf("missing data overstated as coverage: %+v", evidence.OutOfSampleCoverage)
	}
}
