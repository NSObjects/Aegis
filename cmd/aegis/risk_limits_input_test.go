package main

import (
	"Aegis/internal/portfolio"
	"context"
	"strings"
	"testing"
	"time"
)

const validRiskPolicyJSON = `{
	"policy_id":"desk-risk-v1",
	"as_of":"2026-10-08T09:00:00+08:00",
	"scope":"CASH_AND_POSITIONS",
	"currency":"CNY",
	"rules":[
		{"kind":"MAX_SINGLE_POSITION_WEIGHT","limit_bps":3000},
		{"kind":"MAX_INVESTED_WEIGHT","limit_bps":8000}
	]
}`

func TestLoadRiskLimitsExplicitPolicy(t *testing.T) {
	limits,err:=loadRiskLimits(portfolioFixtureFile(t,validRiskPolicyJSON))
	if err!=nil {t.Fatal(err)}
	if limits.PolicyID!="desk-risk-v1" || len(limits.Rules)!=2 ||
		limits.Rules[0].LimitBps==nil || *limits.Rules[0].LimitBps!=3000 {
		t.Fatalf("risk policy not preserved: %+v",limits)
	}
	if err:=portfolio.ValidateLimitsAt(limits,time.Date(2026,10,8,13,0,0,0,
		time.FixedZone("CST",8*60*60)));err!=nil {
		t.Fatal(err)
	}
}

func TestLoadRiskLimitsRejectsMissingAndAmbiguousRules(t *testing.T) {
	for _,tc:=range []struct{name,payload,want string}{
		{"missing policy ID",`{"as_of":"2026-10-08T09:00:00+08:00","scope":"CASH_AND_POSITIONS",
			"currency":"CNY","rules":[{"kind":"MAX_INVESTED_WEIGHT","limit_bps":8000}]}`,"policy_id"},
		{"missing numeric limit",`{"policy_id":"x","as_of":"2026-10-08T09:00:00+08:00",
			"scope":"CASH_AND_POSITIONS","currency":"CNY",
			"rules":[{"kind":"MAX_INVESTED_WEIGHT"}]}`,"explicitly provided"},
		{"null numeric limit",`{"policy_id":"x","as_of":"2026-10-08T09:00:00+08:00",
			"scope":"CASH_AND_POSITIONS","currency":"CNY",
			"rules":[{"kind":"MAX_INVESTED_WEIGHT","limit_bps":null}]}`,"explicitly provided"},
		{"fractional bps",`{"policy_id":"x","as_of":"2026-10-08T09:00:00+08:00",
			"scope":"CASH_AND_POSITIONS","currency":"CNY",
			"rules":[{"kind":"MAX_INVESTED_WEIGHT","limit_bps":2000.5}]}`,"unmarshal"},
		{"unknown rule field",`{"policy_id":"x","as_of":"2026-10-08T09:00:00+08:00",
			"scope":"CASH_AND_POSITIONS","currency":"CNY",
			"rules":[{"kind":"MAX_INVESTED_WEIGHT","limit_bps":8000,"broker_limit":true}]}`,"unknown field"},
		{"duplicate kind",`{"policy_id":"x","as_of":"2026-10-08T09:00:00+08:00",
			"scope":"CASH_AND_POSITIONS","currency":"CNY",
			"rules":[{"kind":"MAX_INVESTED_WEIGHT","limit_bps":8000},
				{"kind":"MAX_INVESTED_WEIGHT","limit_bps":5000}]}`,"duplicated"},
		{"null root",`null`,"must be a JSON object"},
		{"trailing object",validRiskPolicyJSON+` {}`,"exactly one JSON object"},
		{"negative limit",`{"policy_id":"x","as_of":"2026-10-08T09:00:00+08:00",
			"scope":"CASH_AND_POSITIONS","currency":"CNY",
			"rules":[{"kind":"MAX_INVESTED_WEIGHT","limit_bps":-1}]}`,"between 0 and 10000"},
	} {
		t.Run(tc.name,func(t *testing.T){
			_,err:=loadRiskLimits(portfolioFixtureFile(t,tc.payload))
			if err==nil || !strings.Contains(err.Error(),tc.want) {
				t.Fatalf("error=%v, want %q",err,tc.want)
			}
		})
	}
}

func TestRunWithLimitsRejectsBeforeApplicationConfig(t *testing.T) {
	book:=portfolioFixtureFile(t,`{"state":"NOT_PROVIDED"}`)
	future:=portfolioFixtureFile(t,`{
		"policy_id":"future",
		"as_of":"2099-10-08T09:00:00+08:00",
		"scope":"CASH_AND_POSITIONS",
		"currency":"CNY",
		"rules":[{"kind":"MAX_SINGLE_POSITION_WEIGHT","limit_bps":3000}]
	}`)
	err:=runWithLimits(context.Background(),"missing-config.yaml","skills","Asia/Shanghai",
		"600519.SH","",".aegis/evidence",7*24*time.Hour,true,book,0,future)
	if err==nil || !strings.Contains(err.Error(),"after research requested_as_of") {
		t.Fatalf("future policy must be rejected before model/data setup: %v",err)
	}
	err=runWithLimits(context.Background(),"missing-config.yaml","skills","Asia/Shanghai",
		"600519.SH","",".aegis/evidence",7*24*time.Hour,false,"",0,future)
	if err==nil || !strings.Contains(err.Error(),"-risk-limits-file requires -portfolio-review") {
		t.Fatalf("risk policy without review must not be ignored: %v",err)
	}
}

func TestRunWithLimitsRejectsMalformedPolicyBeforeExternalCalls(t *testing.T){
	book:=portfolioFixtureFile(t,`{"state":"NOT_PROVIDED"}`)
	policy:=portfolioFixtureFile(t,`{"policy_id":"x","rules":[]}`)
	err:=runWithLimits(context.Background(),"missing-config.yaml","skills","Asia/Shanghai",
		"600519.SH","",".aegis/evidence",7*24*time.Hour,true,book,0,policy)
	if err==nil || !strings.Contains(err.Error(),"as_of") {
		t.Fatalf("invalid policy must fail before loading missing config: %v",err)
	}
}
