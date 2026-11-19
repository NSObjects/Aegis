package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"

	"Aegis/internal/strategy/pullback"
)

func decodeArchive(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	const maxArchiveBytes = 128 << 20
	limit := &io.LimitedReader{R: file, N: maxArchiveBytes + 1}
	decoder := json.NewDecoder(limit)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF || limit.N == 0 {
		return errors.New("expected one JSON archive within 128 MiB")
	}
	return nil
}

func replayPITFile(path string, out io.Writer) error {
	var snapshot pullback.PITSnapshot
	if err := decodeArchive(path, &snapshot); err != nil {
		return err
	}
	result, err := pullback.ReplayPIT(snapshot)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(result)
}

func backtestFile(path string, out io.Writer) error {
	var input pullback.BacktestInput
	if err := decodeArchive(path, &input); err != nil {
		return err
	}
	report, err := pullback.Backtest(input)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(report)
}
