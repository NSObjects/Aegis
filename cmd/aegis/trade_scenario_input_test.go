package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"Aegis/internal/portfolio"
)

const scenarioPolicyJSON = `{
	"policy_id":"example-policy-v1",
	"as_of":"2020-01-01T00:00:00+08:00",
	"scope":"CASH_AND_POSITIONS","currency":"CNY",
	"rules":[{"kind":"MAX_SINGLE_POSITION_WEIGHT","limit_bps":5000}]
}`

const exampleScenarioJSON = `{
	"scenario_id":"what-if-1",
	"as_of":"2026-10-08T08:00:00+08:00",
	"symbol":"600519.SH","side":"BUY",
	"currency":"CNY","notional_fen":1000
}`

func TestLoadTradeScenarioPreservesExactObservedNotional(t *testing.T) {
	scenario,err:=loadTradeScenario(portfolioFixtureFile(t,exampleScenarioJSON))
	if err!=nil {t.Fatal(err)}
	if scenario.ScenarioID!="what-if-1" || scenario.Side!=portfolio.ScenarioBuy ||
		scenario.NotionalFen==nil || *scenario.NotionalFen!=1000 {
		t.Fatalf("notional is caller-authored exact fen: %+v",scenario)
	}
}

func TestLoadTradeScenarioRejectsUnknownFieldsAndAmbiguity(t *testing.T) {
	for _,tc:=range []struct{name,json,want string}{
		{"null root",`null`,"JSON object"},
		{"notional floating point",`{"scenario_id":"s","notional_fen":10.5}`,"unmarshal"},
		{"unknown price/quantity fields",`{"scenario_id":"s","notional_fen":10,"price":100}`,"unknown field"},
		{"multiple json objects",exampleScenarioJSON+" {}", "exactly one"},
	} {
		t.Run(tc.name,func(t *testing.T){
			_,err:=loadTradeScenario(portfolioFixtureFile(t,tc.json))
			if err==nil || !strings.Contains(err.Error(),tc.want) {
				t.Fatalf("expected %q, got %v",tc.want,err)
			}
		})
	}
}

func TestRunTradeScenarioRequiresExplicitPolicyBeforeConfig(t *testing.T) {
	account:=portfolioFixtureFile(t,`{"state":"NOT_PROVIDED"}`)
	scenario:=portfolioFixtureFile(t,exampleScenarioJSON)
	err:=runWithScenario(context.Background(),"missing-config.yaml","skills","Asia/Shanghai",
		"600519.SH","",".aegis/evidence",7*24*time.Hour,
		true,account,0,"",scenario)
	if err==nil || !strings.Contains(err.Error(),"-risk-limits-file") {
		t.Fatalf("unconstrained scenario may not be treated as allowed: %v",err)
	}
	err=runWithScenario(context.Background(),"missing-config.yaml","skills","Asia/Shanghai",
		"600519.SH","",".aegis/evidence",7*24*time.Hour,
		false,"",0,"",scenario)
	if err==nil || !strings.Contains(err.Error(),"-portfolio-review") {
		t.Fatalf("scenario without account review should fail: %v",err)
	}
}

func TestRunRejectsFutureTradeScenarioBeforeConfigOrModels(t *testing.T) {
	account:=portfolioFixtureFile(t,`{"state":"NOT_PROVIDED"}`)
	policy:=portfolioFixtureFile(t,scenarioPolicyJSON)
	future:=portfolioFixtureFile(t,`{
		"scenario_id":"future-scenario","as_of":"2099-01-01T00:00:00+08:00",
		"symbol":"600519.SH","side":"BUY","currency":"CNY","notional_fen":1000
	}`)
	err:=runWithScenario(context.Background(),"missing-config.yaml","skills","Asia/Shanghai",
		"600519.SH","",".aegis/evidence",7*24*time.Hour,
		true,account,0,policy,future)
	if err==nil || !strings.Contains(err.Error(),"after research requested_as_of") {
		t.Fatalf("future trade scenario must fail before loading missing config: %v",err)
	}
}

func TestRunRejectsNullOrMissingTradeNotionalBeforeConfig(t *testing.T) {
	account:=portfolioFixtureFile(t,`{"state":"NOT_PROVIDED"}`)
	policy:=portfolioFixtureFile(t,scenarioPolicyJSON)
	for _,suffix:=range []string{`"notional_fen":null`,`"scenario_id":"missing-fen"`} {
		scenario:=portfolioFixtureFile(t,`{
			"scenario_id":"notional-case","as_of":"2026-10-08T08:00:00+08:00",
			"symbol":"600519.SH","side":"BUY","currency":"CNY",`+suffix+`}`)
		err:=runWithScenario(context.Background(),"missing-config.yaml","skills","Asia/Shanghai",
			"600519.SH","",".aegis/evidence",7*24*time.Hour,
			true,account,0,policy,scenario)
		if err==nil || !strings.Contains(err.Error(),"notional_fen") {
			t.Fatalf("missing/null nominal is not a real proposal: %v",err)
		}
	}
}
