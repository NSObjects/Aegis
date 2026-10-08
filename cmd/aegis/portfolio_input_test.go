package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Aegis/internal/portfolio"
)

func portfolioFixtureFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "portfolio.json")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadPortfolioContextExplicitStates(t *testing.T) {
	tests := []struct{
		name string
		payload string
		state portfolio.State
	}{
		{"not provided", `{"state":"NOT_PROVIDED"}`, portfolio.StateNotProvided},
		{"observed empty", `{"state":"EMPTY","as_of":"2026-10-08T09:00:00+08:00"}`, portfolio.StateEmpty},
		{"observed positions", `{"state":"POSITIONS","as_of":"2026-10-08T09:00:00+08:00","positions":[{"symbol":"600519.SH","quantity":100}]}`, portfolio.StatePositions},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T){
			got, err := loadPortfolioContext(portfolioFixtureFile(t,tt.payload))
			if err != nil { t.Fatal(err) }
			if got.State!=tt.state {t.Fatalf("got state %s, want %s",got.State,tt.state)}
			if err:=portfolio.Validate(got);err!=nil {t.Fatal(err)}
		})
	}
}

func TestLoadPortfolioContextRejectsAmbiguity(t *testing.T) {
	tests := []struct{name,payload,want string}{
		{"missing state",`{}`,"invalid portfolio state"},
		{"forged cash",`{"state":"NOT_PROVIDED","cash":1000000}`,"unknown field"},
		{"unknown as empty",`{"state":"EMPTY"}`,"as_of is required"},
		{"not provided with holdings",`{"state":"NOT_PROVIDED","positions":[{"symbol":"600519.SH","quantity":100}]}`,"positions must be empty"},
		{"missing quantity",`{"state":"POSITIONS","as_of":"2026-10-08T09:00:00+08:00","positions":[{"symbol":"600519.SH"}]}`,"quantity must be positive"},
		{"trailing object",`{"state":"NOT_PROVIDED"} {"state":"EMPTY"}`,"exactly one JSON value"},
		{"malformed",`{"state":"NOT_PROVIDED"`,"decode portfolio context"},
	}
	for _,tt:=range tests {
		t.Run(tt.name,func(t *testing.T){
			_,err:=loadPortfolioContext(portfolioFixtureFile(t,tt.payload))
			if err==nil || !strings.Contains(err.Error(),tt.want) {
				t.Fatalf("error=%v, want %q",err,tt.want)
			}
		})
	}
}

func TestPortfolioReviewRequiresExplicitInputBeforeResearch(t *testing.T) {
	_,err:=loadPortfolioContext("")
	if err==nil || !strings.Contains(err.Error(), "requires -portfolio-file") {
		t.Fatalf("error=%v",err)
	}
	// Even when the application config is missing, missing portfolio data
	// must be detected first, before any external model or network setup.
	err=run(context.Background(), "missing-config.yaml", "skills", "Asia/Shanghai",
		"600519.SH", "", ".aegis/evidence", 7*24*time.Hour, true, "", 0)
	if err==nil || !strings.Contains(err.Error(),"requires -portfolio-file") {
		t.Fatalf("error=%v",err)
	}
}

func TestRunRejectsStalePortfolioBeforeApplicationConfiguration(t *testing.T) {
	file := portfolioFixtureFile(t,
		`{"state":"POSITIONS","as_of":"2020-01-01T12:00:00+08:00","positions":[{"symbol":"600519.SH","quantity":100}]}`)
	err := run(context.Background(), "missing-config.yaml", "skills", "Asia/Shanghai",
		"600519.SH", "", ".aegis/evidence", 7*24*time.Hour,
		true, file, 24*time.Hour)
	if err == nil || !strings.Contains(err.Error(), "portfolio snapshot is stale") {
		t.Fatalf("stale snapshot must be rejected before config/models: %v", err)
	}
}

func TestRunRejectsInvalidPortfolioAgePolicyBeforeProviderSetup(t *testing.T) {
	file := portfolioFixtureFile(t, `{"state":"NOT_PROVIDED"}`)
	for _, tc := range []struct{
		name string
		review bool
		file string
		age time.Duration
		want string
	}{
		{"negative", true, file, -time.Hour, "must not be negative"},
		{"age without review", false, "", time.Hour, "requires -portfolio-review"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := run(context.Background(), "missing-config.yaml", "skills", "Asia/Shanghai",
				"600519.SH", "", ".aegis/evidence", 7*24*time.Hour,
				tc.review, tc.file, tc.age)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("invalid age policy must fail before provider setup: %v", err)
			}
		})
	}
}

func TestLoadPortfolioContextWithObservedCNYValuation(t *testing.T) {
	book,err:=loadPortfolioContext(portfolioFixtureFile(t,`{
		"state":"POSITIONS",
		"as_of":"2026-10-08T09:00:00+08:00",
		"positions":[{"symbol":"600519.SH","quantity":100}],
		"valuation":{
			"scope":"CASH_AND_POSITIONS",
			"currency":"CNY",
			"total_value_fen":100000,
			"cash_fen":40000,
			"position_values":[{"symbol":"600519.SH","market_value_fen":60000}]
		}
	}`))
	if err!=nil {t.Fatal(err)}
	exposure,err:=portfolio.ComputeExposure(book)
	if err!=nil || exposure==nil || len(exposure.Positions)!=1 ||
		exposure.Positions[0].WeightBps!=6000 {
		t.Fatalf("CLI JSON observed valuation was not preserved: %+v error=%v",exposure,err)
	}
}

func TestLoadPortfolioContextRejectsValuationFabricationOrAmbiguity(t *testing.T) {
	for _,tc:=range []struct{name,payload,want string}{
		{
			"missing one held symbol",
			`{"state":"POSITIONS","as_of":"2026-10-08T09:00:00+08:00",
				"positions":[{"symbol":"600519.SH","quantity":100}],
				"valuation":{"scope":"CASH_AND_POSITIONS","currency":"CNY",
					"cash_fen":40000,"total_value_fen":100000,"position_values":[]}}`,
			"every observed position",
		},
		{
			"money amount cannot be fractional fen",
			`{"state":"EMPTY","as_of":"2026-10-08T09:00:00+08:00",
				"valuation":{"scope":"CASH_AND_POSITIONS","currency":"CNY",
					"cash_fen":1.5,"total_value_fen":2,"position_values":[]}}`,
			"unmarshal",
		},
		{
			"unknown valuation field",
			`{"state":"EMPTY","as_of":"2026-10-08T09:00:00+08:00",
				"valuation":{"scope":"CASH_AND_POSITIONS","currency":"CNY",
					"cash_fen":0,"total_value_fen":0,"broker_cash_fen":10,"position_values":[]}}`,
			"unknown field",
		},
		{
			"unreconciled amount",
			`{"state":"EMPTY","as_of":"2026-10-08T09:00:00+08:00",
				"valuation":{"scope":"CASH_AND_POSITIONS","currency":"CNY",
					"cash_fen":100,"total_value_fen":200,"position_values":[]}}`,
			"does not reconcile",
		},
	} {
		t.Run(tc.name,func(t *testing.T){
			if _,err:=loadPortfolioContext(portfolioFixtureFile(t,tc.payload));
				err==nil || !strings.Contains(err.Error(),tc.want) {
				t.Fatalf("load err=%v, want %q",err,tc.want)
			}
		})
	}
}
