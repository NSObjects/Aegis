// Command aegis-market-canary performs a deliberately small, operator-opted-in
// live TuShare acceptance probe. It never starts a research/scan run, executes
// trades, or persists snapshots outside its own private temporary directory.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"Aegis/internal/capability/technical"
)

const maxCanaryDays = 3

type canaryPlan struct {
	AsOf             string `json:"as_of"`
	Days             int    `json:"days"`
	MaxProviderCalls int    `json:"max_provider_calls"`
	NetworkEnabled   bool   `json:"network_enabled"`
	Message          string `json:"message"`
}

func validCompletedCutoff(asOf string, now time.Time) bool {
	day, err := time.Parse("20060102", asOf)
	if err != nil || day.Format("20060102") != asOf {
		return false
	}
	// Only *previous* calendar dates are eligible for live probes. This
	// avoids accidentally caching an in-progress Shanghai market session.
	shanghaiToday := now.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("20060102")
	return asOf < shanghaiToday
}

func run(ctx context.Context, asOf string, days int, execute bool, token string, output io.Writer, now time.Time) error {
	if ctx == nil || output == nil {
		return errors.New("canary requires context and output writer")
	}
	if days < 1 || days > maxCanaryDays {
		return errors.New("--days must be 1..3")
	}
	if !validCompletedCutoff(asOf, now) {
		return errors.New("--as-of must be a completed Shanghai calendar date, YYYYMMDD, before today")
	}
	plan := canaryPlan{
		AsOf: asOf,
		Days: days,
		MaxProviderCalls: 4+2*days,
		NetworkEnabled: execute,
		Message: "Read-only TuShare provider validation; no research runs, orders, production cache or stored market data",
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if !execute {
		plan.Message = "Dry run only: no TuShare network calls. Use --execute and TUSHARE_TOKEN to run deliberately."
		return encoder.Encode(plan)
	}
	if strings.TrimSpace(token) == "" {
		return errors.New("--execute requires TUSHARE_TOKEN in the local environment")
	}
	client, err := technical.NewCanaryTushareClient(token)
	if err != nil {
		return errors.New("invalid local TuShare token configuration")
	}
	// Never reuse or modify production market-day-cache (or scan snapshots).
	// TempDir is user-private and is removed after successful OR failed runs.
	tempDir, err := os.MkdirTemp("", "aegis-tushare-canary-")
	if err != nil {
		return fmt.Errorf("cannot create private probe scratch directory: %w", err)
	}
	defer os.RemoveAll(tempDir)
	cache, err := technical.NewMarketDayCache(client, tempDir)
	if err != nil {
		return fmt.Errorf("cannot initialize private probe cache: %w", err)
	}
	probe, err := cache.Probe(ctx, asOf, days)
	if err != nil {
		return fmt.Errorf("live market canary failed (no scan was started): %s",
			strings.ReplaceAll(err.Error(), token, "[REDACTED]"))
	}
	if err := encoder.Encode(probe); err != nil {
		return err
	}
	if !probe.ReadyForFullPool {
		return errors.New("market canary detected severe undercoverage; do not start a full scan")
	}
	return nil
}

func main() {
	asOf := flag.String("as-of", "", "completed Shanghai trading day (YYYYMMDD); today's session is not allowed")
	days := flag.Int("days", 1, "number of completed market sessions to sample (1..3)")
	execute := flag.Bool("execute", false, "explicitly allow bounded live TuShare API requests (default: dry run)")
	timeout := flag.Duration("timeout", 2*time.Minute, "maximum time for the entire probe (15s..3m)")
	flag.Parse()
	if *timeout < 15*time.Second || *timeout > 3*time.Minute {
		fmt.Fprintln(os.Stderr, "--timeout must be 15s..3m")
		os.Exit(2)
	}
	// Do not even access the credential for a dry run.
	token := ""
	if *execute {
		token = os.Getenv("TUSHARE_TOKEN")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	if err := run(ctx, *asOf, *days, *execute, token, os.Stdout, time.Now()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
