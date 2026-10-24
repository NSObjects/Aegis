package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"Aegis/internal/application"
)

// webScheduleRunner drives the existing engines from scheduler tasks. The
// research trigger carries a deterministic Idempotency-Key derived from the
// task and trading day, so even a duplicated trigger cannot start a second
// paid run for the same day.
type webScheduleRunner struct {
	scans    *application.ScanManager
	runs     *application.RunManager
	mode     application.ResearchMode
	lookback time.Duration
}

func (r *webScheduleRunner) RunScan(ctx context.Context, task application.ScheduleTask, tradeDate string) (string, error) {
	if r.scans == nil {
		return "", errors.New("scan engine unavailable")
	}
	record, err := r.scans.Submit(ctx, tradeDate, task.UniverseSource, task.Symbols)
	if err != nil {
		return "", err
	}
	return record.ID, nil
}

func (r *webScheduleRunner) RunResearch(ctx context.Context, task application.ScheduleTask, tradeDate string) (string, error) {
	input, err := application.NewLiveResearchInput(task.Symbol, "", time.Now().UTC(), r.lookback)
	if err != nil {
		return "", err
	}
	request := application.ResearchRequest{
		IdempotencyKey: fmt.Sprintf("sched-%s-%s", task.ID, tradeDate),
		Mode:           r.mode,
		Input:          input,
	}
	if r.mode == application.ModePortfolio {
		request.Portfolio = nil
	}
	rec, err := r.runs.Submit(ctx, request)
	if err != nil {
		return "", err
	}
	return rec.ID, nil
}

// webScheduleTracker maps engine objects back to terminal execution state.
type webScheduleTracker struct {
	scans *application.ScanManager
	runs  *application.RunManager
}

func (t *webScheduleTracker) RefTerminal(ctx context.Context, task application.ScheduleTask, refID string) (bool, bool, string, error) {
	switch task.Kind {
	case application.ScheduleKindScan:
		record, err := t.scans.Get(refID)
		if err != nil {
			return false, false, "", err
		}
		switch record.State {
		case application.ScanCompleted:
			return true, false, "", nil
		case application.ScanFailed:
			return true, true, record.FailureCode, nil
		}
		return false, false, "", nil
	case application.ScheduleKindResearch:
		record, err := t.runs.Get(ctx, refID)
		if err != nil {
			return false, false, "", err
		}
		switch record.State {
		case application.RunCompleted:
			return true, false, "", nil
		case application.RunFailed:
			return true, true, record.FailureCode, nil
		case application.RunCancelled:
			return true, true, "RESEARCH_CANCELLED", nil
		}
		return false, false, "", nil
	}
	return false, false, "", errors.New("unknown schedule task kind")
}
