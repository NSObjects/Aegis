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
		"600519.SH", "", ".aegis/evidence", 7*24*time.Hour, true, "")
	if err==nil || !strings.Contains(err.Error(),"requires -portfolio-file") {
		t.Fatalf("error=%v",err)
	}
}
