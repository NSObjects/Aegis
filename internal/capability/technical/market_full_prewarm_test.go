package technical

import (
	"context"
	"strings"
	"testing"
)

func TestFullMarketPrewarmReusesProductionPreflightAnd184RequestBudget(t *testing.T) {
	fixture:=&marketProbeFixture{universeSize:2}
	cache,closeServer:=probeFixtureCache(t,fixture)
	defer closeServer()
	progress:=0
	report,err:=cache.ProbeFullPrewarm(context.Background(),"20261022",func(done,total int)error{
		if total!=90||done<1||done>total||done<progress{
			t.Fatalf("unexpected full prewarm progress %d/%d",done,total)
		}
		progress=done
		return nil
	})
	if err!=nil{t.Fatal(err)}
	if !report.ReadyForPoolReview||report.MainBoardSymbols!=2||
		report.SessionsVerified!=90||report.DailySnapshots!=90||report.FactorSnapshots!=90||
		report.LastSession!="20261022"||report.FirstSession==""||
		report.Contiguous65Symbols!=2||report.Contiguous65WithFactors!=2||
		report.MissingFactor65Symbols!=0||report.GapOrShort65Symbols!=0||
		report.MinDailyMembers!=2||report.MinFactorMembers!=2||progress!=90||
		report.DailySnapshotBytes<=0||report.FactorSnapshotBytes<=0 {
		t.Fatalf("unexpected full prewarm acceptance %+v",report)
	}
	calls:=fixture.count()
	if report.ExpectedProviderCalls!=184||len(calls)!=184 {
		t.Fatalf("prewarm request bound wrong: report=%d actual=%d",report.ExpectedProviderCalls,len(calls))
	}
	count:=map[string]int{}
	for _,api:=range calls{count[api]++}
	for name,n:=range map[string]int{"stock_basic":3,"trade_cal":1,"daily":90,"adj_factor":90} {
		if count[name]!=n{t.Fatalf("%s calls=%d expected=%d",name,count[name],n)}
	}
}

func TestFullMarketPrewarmFailsOnIncompleteHistoricalCrossSection(t *testing.T) {
	fixture:=&marketProbeFixture{universeSize:100}
	cache,done:=probeFixtureCache(t,fixture)
	defer done()
	progress:=0
	_,err:=cache.ProbeFullPrewarm(context.Background(),"20261022",func(done,total int)error{progress=done;return nil})
	if err==nil||!strings.Contains(err.Error(),"insufficient pool coverage") {
		t.Fatalf("truncated full market history was accepted: %v",err)
	}
	if progress!=0||len(fixture.count())!=6 {
		t.Fatalf("failed prewarm continued after first bad day: progress=%d calls=%v",progress,fixture.count())
	}
}

func TestFullMarketPrewarmRejectsInvalidDateBeforeProvider(t *testing.T) {
	fixture:=&marketProbeFixture{universeSize:2}
	cache,done:=probeFixtureCache(t,fixture)
	defer done()
	_,err:=cache.ProbeFullPrewarm(context.Background(),"20261399",nil)
	if err==nil||len(fixture.count())!=0 {
		t.Fatalf("invalid full prewarm cutoff reached provider: %v %v",err,fixture.count())
	}
}
