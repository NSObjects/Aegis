package portfolio

import (
	"math"
	"strings"
	"testing"
	"time"
)

func tradeScenario(side ScenarioSide, symbol string, fen int64) *TradeScenario {
	return &TradeScenario{
		ScenarioID:"hypothetical-001",
		AsOf:"2026-10-08T10:00:00+08:00",
		Symbol:symbol, Side:side, Currency:CurrencyCNY, NotionalFen:&fen,
	}
}

func postPolicy(single,invested int64) *Limits {
	return testLimits(
		LimitRule{Kind:LimitMaxSinglePositionWeight,LimitBps:&single},
		LimitRule{Kind:LimitMaxInvestedWeight,LimitBps:&invested},
	)
}

func TestBuyScenarioTransfersCashToExistingPositionAtExactLimits(t *testing.T) {
	book:=observedBook() // 600519=60000, cash=30000, total=100000
	res,err:=EvaluateTradeScenario(book,postPolicy(7000,8000),
		tradeScenario(ScenarioBuy,"600519.SH",10000),testCutoff(),"600519.SH")
	if err!=nil {t.Fatal(err)}
	if res.Status!=ScenarioWithinLimits || res.ProjectedCashFen==nil || *res.ProjectedCashFen!=20000 ||
		res.ProjectedInvestedFen==nil || *res.ProjectedInvestedFen!=80000 ||
		len(res.Checks)!=2 || res.Checks[0].Symbol!="600519.SH" ||
		res.Checks[0].ProjectedWeightBps!=7000 ||
		res.Checks[1].ProjectedWeightBps!=8000 {
		t.Fatalf("exactly-at-limit modeled transfer is not an order approval: %+v",res)
	}
	if book.Valuation.CashFen!=30000 || book.Valuation.PositionValues[1].MarketValueFen!=60000 {
		t.Fatal("hypothetical scenario must never mutate the observed account")
	}
}

func TestScenarioExactBreachNotHiddenByDisplayTruncation(t *testing.T) {
	book:=observedBook()
	res,err:=EvaluateTradeScenario(book,postPolicy(7000,8000),
		tradeScenario(ScenarioBuy,"600519.SH",10001),testCutoff(),"600519.SH")
	if err!=nil {t.Fatal(err)}
	if res.Status!=ScenarioBreach || len(res.Checks)!=2 ||
		res.Checks[0].Status!=LimitReviewBreach || res.Checks[1].Status!=LimitReviewBreach ||
		res.Checks[0].ProjectedWeightBps!=7000 || false {
		t.Fatalf("7000.1 bps still breaches 7000 despite display truncation: %+v",res)
	}
}

func TestSellScenarioRebalancesHypotheticalAmountsWithoutInventingShares(t *testing.T) {
	book:=observedBook()
	res,err:=EvaluateTradeScenario(book,postPolicy(5000,6000),
		tradeScenario(ScenarioSell,"600519.SH",20000),testCutoff(),"600519.SH")
	if err!=nil {t.Fatal(err)}
	if res.Status!=ScenarioWithinLimits || *res.ProjectedCashFen!=50000 ||
		*res.ProjectedInvestedFen!=50000 ||
		res.Checks[0].ProjectedWeightBps!=4000 || res.Checks[1].ProjectedWeightBps!=5000 {
		t.Fatalf("cash/value-only modeled sale incorrect: %+v",res)
	}
}

func TestBuyNewPositionIncludedInHighestExposureCheck(t *testing.T) {
	book:=observedBook()
	res,err:=EvaluateTradeScenario(book,postPolicy(5000,8000),
		tradeScenario(ScenarioBuy,"300750.SZ",10000),testCutoff(),"300750.SZ")
	if err!=nil {t.Fatal(err)}
	if res.Status!=ScenarioBreach ||
		res.Checks[0].Symbol!="600519.SH" || res.Checks[0].ProjectedWeightBps!=6000 ||
		res.Checks[1].ProjectedWeightBps!=8000 || false {
		t.Fatalf("new hypothetical symbol must not erase existing concentrated positions: %+v",res)
	}
}

func TestScenarioInsufficientFundsAndPositionMarketValue(t *testing.T) {
	for _,tc:=range []struct{side ScenarioSide; symbol string; amount int64; want ScenarioStatus}{
		{ScenarioBuy,"600519.SH",30001,ScenarioInsufficientCash},
		{ScenarioSell,"600519.SH",60001,ScenarioInsufficientValue},
		{ScenarioSell,"300750.SZ",100,ScenarioInsufficientValue},
	} {
		book:=observedBook()
		res,err:=EvaluateTradeScenario(book,postPolicy(10000,10000),
			tradeScenario(tc.side,tc.symbol,tc.amount),testCutoff(),tc.symbol)
		if err!=nil || res.Status!=tc.want ||
			res.ProjectedCashFen!=nil || len(res.Checks)!=0 || false {
			t.Fatalf("%s/%s/%d: result=%+v err=%v",tc.side,tc.symbol,tc.amount,res,err)
		}
	}
}

func TestScenarioInsufficientDataNeverBecomesWithinLimits(t *testing.T) {
	for _,book:=range []*Context{
		{State:StateNotProvided},
		{State:StateEmpty,AsOf:"2026-10-08T09:00:00+08:00"},
		{State:StateEmpty,AsOf:"2026-10-08T09:00:00+08:00",
			Valuation:&Valuation{Scope:ValuationScopeCashAndPositions,
				Currency:CurrencyCNY,TotalValueFen:0,CashFen:0,PositionValues:[]PositionValue{}}},
	} {
		result,err:=EvaluateTradeScenario(book,postPolicy(10000,10000),
			tradeScenario(ScenarioBuy,"600519.SH",1),testCutoff(),"600519.SH")
		if err!=nil || result.Status!=ScenarioInsufficientData ||
			result.ProjectedCashFen!=nil || len(result.Checks)!=0 || false {
			t.Fatalf("unknown portfolio cannot produce a projection: %+v err=%v",result,err)
		}
	}
}

func TestTradeScenarioValidationsFailBeforeProjection(t *testing.T) {
	cases:=[]struct{
		name string
		edit func(*TradeScenario)
		want string
	}{
		{"empty scenario ID",func(s *TradeScenario){s.ScenarioID=""},"scenario_id"},
		{"missing time",func(s *TradeScenario){s.AsOf=""},"RFC3339"},
		{"future time",func(s *TradeScenario){s.AsOf="2026-10-08T13:00:00+08:00"},"after research"},
		{"stale to account",func(s *TradeScenario){s.AsOf="2026-10-08T08:59:59+08:00"},"precedes observed account"},
		{"earlier than policy",func(s *TradeScenario){s.AsOf="2026-10-08T07:00:00+08:00"},"precedes risk policy"},
		{"wrong symbol",func(s *TradeScenario){s.Symbol="000001.SZ"},"does not match research"},
		{"lowercase symbol",func(s *TradeScenario){s.Symbol="600519.sh"},"does not match research"},
		{"unsupported side",func(s *TradeScenario){s.Side="SHORT"},"unsupported"},
		{"wrong currency",func(s *TradeScenario){s.Currency="USD"},"currency"},
		{"missing notional",func(s *TradeScenario){s.NotionalFen=nil},"explicitly provided"},
		{"zero amount",func(s *TradeScenario){zero:=int64(0);s.NotionalFen=&zero},"positive"},
		{"negative amount",func(s *TradeScenario){negative:=int64(-1);s.NotionalFen=&negative},"positive"},
	}
	for _,tc:=range cases {
		t.Run(tc.name,func(t *testing.T){
			s:=tradeScenario(ScenarioBuy,"600519.SH",100)
			tc.edit(s)
			err:=ValidateTradeScenarioAt(s,observedBook(),postPolicy(10000,10000),testCutoff(),"600519.SH")
			if err==nil || !strings.Contains(err.Error(),tc.want) {
				t.Fatalf("expected %q, got %v",tc.want,err)
			}
		})
	}
}

func TestScenarioRequiresExplicitRiskPolicyAndReconciledPortfolio(t *testing.T) {
	s:=tradeScenario(ScenarioBuy,"600519.SH",100)
	if err:=ValidateTradeScenarioAt(s,observedBook(),nil,testCutoff(),"600519.SH");
		err==nil || !strings.Contains(err.Error(),"risk policy") {
		t.Fatalf("no policy cannot be treated as unconstrained: %v",err)
	}
	book:=observedBook()
	book.Valuation.TotalValueFen=math.MaxInt64
	if _,err:=EvaluateTradeScenario(book,postPolicy(10000,10000),
		s,testCutoff(),"600519.SH");err==nil || !strings.Contains(err.Error(),"reconcile") {
		t.Fatalf("inconsistent money may not be projected: %v",err)
	}
}

func TestNotApplicableScenarioNeverIncludesProjection(t *testing.T) {
	s:=tradeScenario(ScenarioBuy,"600519.SH",100)
	out:=BuildNotApplicableScenario(s,postPolicy(10000,10000),"intent is MONITOR")
	if out.Status!=ScenarioNotApplicable || out.ProjectedCashFen!=nil ||
		len(out.Checks)!=0 || false {
		t.Fatalf("non-candidate must not project: %+v",out)
	}
}

func TestScenarioExactTimeZoneComparison(t *testing.T) {
	s:=tradeScenario(ScenarioBuy,"600519.SH",100)
	s.AsOf="2026-10-08T09:00:00+09:00" // 08:00 +08: earlier than observed account at 09:00 +08
	if err:=ValidateTradeScenarioAt(s,observedBook(),postPolicy(10000,10000),testCutoff(),"600519.SH");
		err==nil || !strings.Contains(err.Error(),"precedes observed account") {
		t.Fatalf("time zones must be normalized to instants: %v",err)
	}
	s.AsOf="2026-10-08T11:00:00+08:00"
	if err:=ValidateTradeScenarioAt(s,observedBook(),postPolicy(10000,10000),
		time.Date(2026,10,8,11,0,0,0,time.FixedZone("CST",8*3600)),"600519.SH");err!=nil {
		t.Fatal(err)
	}
}
