package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"runtime/debug"

	"Aegis/internal/strategy/pullback"
)

type r6Evidence struct {
	Version             string                    `json:"evidence_version"`
	ArchiveSHA256       string                    `json:"archive_sha256"`
	ReportSHA256        string                    `json:"report_sha256"`
	Build               r6Build                   `json:"build"`
	Repeatable          bool                      `json:"repeatable"`
	ArchiveProvenance   string                    `json:"archive_provenance"`
	ReviewStatus        string                    `json:"review_status"`
	OutOfSampleCoverage []r6WindowEvidence        `json:"out_of_sample_coverage"`
	Report              pullback.ValidationReport `json:"report"`
}

type r6Build struct {
	GoVersion string `json:"go_version,omitempty"`
	Revision  string `json:"revision,omitempty"`
	Modified  *bool  `json:"modified,omitempty"`
}

type r6WindowEvidence struct {
	From         string `json:"from"`
	Through      string `json:"through"`
	SignalDays   int    `json:"signal_days"`
	Candidates   int    `json:"candidates"`
	Insufficient int    `json:"insufficient_candidates"`
	ClosedTrades int    `json:"closed_trades"`
	Labeled      int    `json:"independent_labels"`
}

func r6EvidenceFile(path string, out io.Writer) error {
	first, inputDigest, err := replayR6Evidence(path)
	if err != nil {
		return err
	}
	second, repeatedDigest, err := replayR6Evidence(path)
	if err != nil {
		return err
	}
	initial, err := json.Marshal(first)
	if err != nil {
		return err
	}
	repeated, err := json.Marshal(second)
	if err != nil {
		return err
	}
	if inputDigest != repeatedDigest || !bytes.Equal(initial, repeated) {
		return errors.New("archive or validation output changed during repeated replay")
	}
	hash := sha256.Sum256(initial)
	evidence := r6Evidence{
		Version: "r6-replay-evidence-v1", ArchiveSHA256: inputDigest, ReportSHA256: hex.EncodeToString(hash[:]),
		Build: r6BuildMetadata(), Repeatable: true, ArchiveProvenance: "UNVERIFIED",
		ReviewStatus: "PENDING_INDEPENDENT_REVIEW", Report: first,
		OutOfSampleCoverage: make([]r6WindowEvidence, 0, len(first.Folds)),
	}
	for _, fold := range first.Folds {
		test := fold.OutOfSample
		row := r6WindowEvidence{
			From: fold.Range.TestFrom, Through: fold.Range.TestThrough, SignalDays: len(test.Replay.Signals),
			Candidates: test.Metrics.Candidates, Insufficient: test.Metrics.MissingCandidates,
			ClosedTrades: test.Metrics.Closed,
		}
		if test.Metrics.Classification != nil {
			row.Labeled = test.Metrics.Classification.Labeled
		}
		evidence.OutOfSampleCoverage = append(evidence.OutOfSampleCoverage, row)
	}
	return json.NewEncoder(out).Encode(evidence)
}

func replayR6Evidence(path string) (pullback.ValidationReport, string, error) {
	var input pullback.ValidationInput
	digest := sha256.New()
	if err := decodeArchiveTo(path, &input, digest); err != nil {
		return pullback.ValidationReport{}, "", err
	}
	report, err := pullback.ValidateBacktest(input)
	return report, hex.EncodeToString(digest.Sum(nil)), err
}

func r6BuildMetadata() r6Build {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return r6Build{}
	}
	result := r6Build{GoVersion: info.GoVersion}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			result.Revision = setting.Value
		case "vcs.modified":
			modified := setting.Value == "true"
			result.Modified = &modified
		}
	}
	return result
}
