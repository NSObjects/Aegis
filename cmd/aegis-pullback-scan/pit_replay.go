package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"

	"Aegis/internal/strategy/pullback"
)

func replayPITFile(path string, out io.Writer) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	const maxSnapshotBytes = 128 << 20
	limit := &io.LimitedReader{R: file, N: maxSnapshotBytes + 1}
	decoder := json.NewDecoder(limit)
	decoder.DisallowUnknownFields()
	var snapshot pullback.PITSnapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF || limit.N == 0 {
		return errors.New("expected one PIT snapshot within 128 MiB")
	}
	result, err := pullback.ReplayPIT(snapshot)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(result)
}
