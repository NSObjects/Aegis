package portfolio

import (
	"math"
	"strings"
	"testing"
	"time"
)

func bps(value int64) *int64 { return &value }

func testLimits(rules ...LimitRule) *Limits {
	return &Limits{
		PolicyID:"client-short-term-v1",
		AsOf:"2026-10-08T08:00:00+08:00",
		Scope:ValuationScopeCashAndPositions,
		Currency:CurrencyCNY,
		Rules:rules,
	}
}

func testCutoff() time.Time {
	return time.Date(2026,10,8,11,0,0,0,time.FixedZone("CST",8*60*60))
}

func TestEvaluateCurrentLimitsDetectsIndependentBreaches(t *testing.T) {
	book:=observedBook() // cash 30%, symbol 600519 = 60%, invested = 70%
	policy:=testLimits(
		LimitRule{Kind:LimitMaxSinglePositionWeight,LimitBps:bps(5000)},
		LimitRule{Kind:LimitMaxInvestedWeight,LimitBps:bps(6500)},
	)
	review,err:=EvaluateCurrentLimits(book,policy,testCutoff())
	if err!=nil {t.Fatal(err)}
	if review.Status!=LimitReviewBreach || review.PolicyID!=policy.PolicyID ||
		review.PolicyAsOf!=policy.AsOf || review.AccountAsOf!=book.AsOf ||
		len(review.Checks)!=2 {
		t.Fatalf("wrong current policy status: %+v",review)
	}
	if review.Checks[0].Kind!=LimitMaxSinglePositionWeight ||
		review.Checks[0].Symbol!="600519.SH" || review.Checks[0].ObservedWeightBps==nil ||
		*review.Checks[0].ObservedWeightBps!=6000 ||
		review.Checks[0].Status!=LimitReviewBreach {
		t.Fatalf("single-position breach incorrectly detected: %+v",review.Checks[0])
	}
	if review.Checks[1].ObservedWeightBps==nil ||
		*review.Checks[1].ObservedWeightBps!=7000 ||
		review.Checks[1].Status!=LimitReviewBreach {
		t.Fatalf("invested-weight breach incorrectly detected: %+v",review.Checks[1])
	}
}

func TestEvaluateCurrentLimitsExactBoundaryAndNoRoundingBypass(t *testing.T) {
	book:=&Context{
		State:StatePositions,AsOf:"2026-10-08T09:00:00+08:00",
		Positions:[]Position{{Symbol:"600519.SH",Quantity:100}},
		Valuation:&Valuation{
			Scope:ValuationScopeCashAndPositions,Currency:CurrencyCNY,
			TotalValueFen:1000000,CashFen:699999,
			PositionValues:[]PositionValue{{Symbol:"600519.SH",MarketValueFen:300001}},
		},
	}
	limit:=testLimits(LimitRule{Kind:LimitMaxSinglePositionWeight,LimitBps:bps(3000)})
	review,err:=EvaluateCurrentLimits(book,limit,testCutoff())
	if err!=nil {t.Fatal(err)}
	if review.Status!=LimitReviewBreach ||
		review.Checks[0].ObservedWeightBps==nil ||
		*review.Checks[0].ObservedWeightBps!=3000 {
		t.Fatalf("3000.01 bps must breach even if displayed as 3000: %+v",review)
	}
	book.Valuation.CashFen++
	book.Valuation.PositionValues[0].MarketValueFen--
	review,err=EvaluateCurrentLimits(book,limit,testCutoff())
	if err!=nil || review.Status!=LimitReviewPass {
		t.Fatalf("exactly 3000 bps is not a breach: %+v err=%v",review,err)
	}
}

func TestEvaluateCurrentLimitsNoValuationIsNeverPass(t *testing.T) {
	limit:=testLimits(LimitRule{Kind:LimitMaxSinglePositionWeight,LimitBps:bps(0)})
	for _,book:=range []*Context{
		{State:StateNotProvided},
		{State:StateEmpty,AsOf:"2026-10-08T09:00:00+08:00"},
		{State:StatePositions,AsOf:"2026-10-08T09:00:00+08:00",
			Positions:[]Position{{Symbol:"600519.SH",Quantity:100}}},
		{State:StateEmpty,AsOf:"2026-10-08T09:00:00+08:00",
			Valuation:&Valuation{
				Scope:ValuationScopeCashAndPositions,Currency:CurrencyCNY,
				CashFen:0,TotalValueFen:0,PositionValues:[]PositionValue{},
			}},
	} {
		result,err:=EvaluateCurrentLimits(book,limit,testCutoff())
		if err!=nil || result.Status!=LimitReviewInsufficientData ||
			result.Checks[0].Status!=LimitReviewInsufficientData ||
			result.Checks[0].ObservedWeightBps!=nil {
			t.Fatalf("unknown/undefined denominator must not pass: %+v err=%v",result,err)
		}
	}
}

func TestEvaluateCurrentLimitsEmptyKnownValuedAccount(t *testing.T) {
	book:=&Context{
		State:StateEmpty,AsOf:"2026-10-08T09:00:00+08:00",
		Valuation:&Valuation{Scope:ValuationScopeCashAndPositions,Currency:CurrencyCNY,
			CashFen:100000,TotalValueFen:100000,PositionValues:[]PositionValue{},
		},
	}
	review,err:=EvaluateCurrentLimits(book,testLimits(
		LimitRule{Kind:LimitMaxSinglePositionWeight,LimitBps:bps(0)},
		LimitRule{Kind:LimitMaxInvestedWeight,LimitBps:bps(0)},
	),testCutoff())
	if err!=nil || review.Status!=LimitReviewPass {
		t.Fatalf("known cash-only portfolio must pass both zero invested limits: %+v err=%v",review,err)
	}
	if review.Checks[0].Symbol!="" || *review.Checks[0].ObservedWeightBps!=0 {
		t.Fatalf("empty portfolio maximum weight must be zero: %+v",review.Checks[0])
	}
}

func TestEvaluateCurrentLimitsAvoidsInt64MultiplicationOverflow(t *testing.T) {
	value:=int64(math.MaxInt64/2)
	book:=&Context{
		State:StatePositions, AsOf:"2026-10-08T09:00:00+08:00",
		Positions:[]Position{{Symbol:"600519.SH",Quantity:1}},
		Valuation:&Valuation{Scope:ValuationScopeCashAndPositions,Currency:CurrencyCNY,
			TotalValueFen:value*2, CashFen:value,
			PositionValues:[]PositionValue{{Symbol:"600519.SH",MarketValueFen:value}},
		},
	}
	review,err:=EvaluateCurrentLimits(book,
		testLimits(LimitRule{Kind:LimitMaxSinglePositionWeight,LimitBps:bps(4999)}),testCutoff())
	if err!=nil || review.Status!=LimitReviewBreach {
		t.Fatalf("large values need arbitrary-precision comparison: %+v err=%v",review,err)
	}
}

func TestValidateLimitsContract(t *testing.T) {
	tests:=[]struct{
		name string
		modify func(*Limits)
		want string
	}{
		{"no policy ID",func(l *Limits){l.PolicyID="  "},"policy_id"},
		{"no as of",func(l *Limits){l.AsOf=""},"as_of"},
		{"invalid as of",func(l *Limits){l.AsOf="2026-10-08"},"RFC3339"},
		{"currency",func(l *Limits){l.Currency="USD"},"currency"},
		{"scope",func(l *Limits){l.Scope="ALL_ASSETS"},"scope"},
		{"no rules",func(l *Limits){l.Rules=nil},"at least one rule"},
		{"unknown rule",func(l *Limits){l.Rules[0].Kind="MAX_LEVERAGE"},"unsupported kind"},
		{"duplicate rule",func(l *Limits){l.Rules=append(l.Rules,l.Rules[0])},"duplicated"},
		{"missing explicit bps",func(l *Limits){l.Rules[0].LimitBps=nil},"explicitly provided"},
		{"negative",func(l *Limits){l.Rules[0].LimitBps=bps(-1)},"between 0 and 10000"},
		{"above 100%",func(l *Limits){l.Rules[0].LimitBps=bps(10001)},"between 0 and 10000"},
		{"future policy",func(l *Limits){l.AsOf="2026-10-08T12:00:00+08:00"},"after research requested_as_of"},
	}
	for _,tc:=range tests {
		t.Run(tc.name,func(t *testing.T){
			limit:=testLimits(LimitRule{Kind:LimitMaxSinglePositionWeight,LimitBps:bps(3000)})
			tc.modify(limit)
			if err:=ValidateLimitsAt(limit,testCutoff());err==nil || !strings.Contains(err.Error(),tc.want) {
				t.Fatalf("error=%v, want %q",err,tc.want)
			}
		})
	}
	if _,err:=EvaluateCurrentLimits(observedBook(),nil,testCutoff());err!=nil {
		t.Fatalf("optional policy must remain backwards compatible: %v",err)
	}
}
