package technical

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FullMarketPrewarmReport is an offline-inspectable aggregate summary of one
// 90-session preflight. It excludes raw prices, symbols and API credentials.
// Full cache verification does NOT prove historical as-of provider revisions.
type FullMarketPrewarmReport struct {
	AsOf                    string   `json:"as_of"`
	UniverseSource          string   `json:"universe_source"`
	MainBoardSymbols        int      `json:"main_board_symbols"`
	ExpectedProviderCalls   int      `json:"max_provider_calls"`
	SessionsVerified        int      `json:"sessions_verified"`
	FirstSession            string   `json:"first_session"`
	LastSession             string   `json:"last_session"`
	DailySnapshots          int      `json:"daily_snapshots"`
	FactorSnapshots         int      `json:"factor_snapshots"`
	DailySnapshotBytes      int64    `json:"daily_snapshot_bytes"`
	FactorSnapshotBytes     int64    `json:"factor_snapshot_bytes"`
	MinDailyMembers         int      `json:"min_main_board_daily"`
	MinFactorMembers        int      `json:"min_main_board_factors"`
	Contiguous65Symbols     int      `json:"contiguous_65_symbols"`
	Contiguous65WithFactors int      `json:"contiguous_65_with_factors"`
	GapOrShort65Symbols     int      `json:"gap_or_short_65_symbols"`
	MissingFactor65Symbols  int      `json:"missing_factor_65_symbols"`
	ReadyForPoolReview      bool     `json:"ready_for_pool_review"`
	Warnings                []string `json:"warnings"`
}

// ProbeFullPrewarm reuses the PRODUCTION pool cache preflight, not a second
// simplified data path. With an initially empty private scratch directory,
// at most 3 stock_basic + 1 trade_cal + 2*90 market-date API requests occur.
// On failure no unverified stock results are published, and callers remove
// the scratch directory. The caller MUST use a new isolated directory.
func (c *MarketDayCache) ProbeFullPrewarm(ctx context.Context, asOf string, progress func(int,int) error) (FullMarketPrewarmReport,error) {
	out:=FullMarketPrewarmReport{
		AsOf:asOf,ExpectedProviderCalls:4+2*marketDaySessions,
		Warnings:[]string{},
	}
	if c==nil || c.api==nil || ctx==nil || !withinPoolWindow(asOf,asOf,asOf) {
		return out,errors.New("full prewarm requires initialized cache, context and valid cutoff")
	}
	universe,err:=c.api.ListStockUniverse(ctx,asOf)
	if err!=nil{return out,fmt.Errorf("prewarm PIT universe: %w",err)}
	out.UniverseSource=universe.Source
	out.MainBoardSymbols=len(universe.Symbols)
	if out.MainBoardSymbols==0{return out,errors.New("empty frozen main-board universe")}

	prepared,err:=c.Prepare(ctx,asOf,universe.Symbols,progress)
	if err!=nil{return out,fmt.Errorf("90-session production prewarm rejected: %w",err)}
	// Read the exact snapshots just written, without another provider call.
	// Files are written only by readDate and named for their API and date.
	entries,err:=os.ReadDir(c.dir)
	if err!=nil{return out,err}
	dates:=make([]string,0,marketDaySessions)
	for _,entry:=range entries {
		name:=entry.Name()
		if strings.HasPrefix(name,"daily_")&&strings.HasSuffix(name,"_v1.json") {
			date:=strings.TrimSuffix(strings.TrimPrefix(name,"daily_"),"_v1.json")
			if !withinPoolWindow(date,date,date){return out,errors.New("invalid prewarm snapshot name")}
			dates=append(dates,date)
		}
	}
	sort.Strings(dates)
	if len(dates)!=marketDaySessions{return out,fmt.Errorf("expected %d daily cache dates, got %d",marketDaySessions,len(dates))}
	out.FirstSession=dates[0]
	out.LastSession=dates[len(dates)-1]
	if out.LastSession!=asOf {return out,errors.New("prewarm cache does not end at requested cutoff")}
	out.MinDailyMembers=len(universe.Symbols)
	out.MinFactorMembers=len(universe.Symbols)
	members:=make(map[string]struct{},len(universe.Symbols))
	for _,symbol:=range universe.Symbols{members[symbol]=struct{}{}}
	for _,date:=range dates {
		if err:=ctx.Err();err!=nil{return out,err}
		daily,err:=c.readDate(ctx,"daily",date)
		if err!=nil{return out,fmt.Errorf("recheck daily %s: %w",date,err)}
		factor,err:=c.readDate(ctx,"adj_factor",date)
		if err!=nil{return out,fmt.Errorf("recheck factors %s: %w",date,err)}
		dailyN,factorN:=0,0
		for _,row:=range daily.Prices{if _,ok:=members[row.Symbol];ok{dailyN++}}
		for _,row:=range factor.Factors{if _,ok:=members[row.Symbol];ok{factorN++}}
		if dailyN<out.MinDailyMembers{out.MinDailyMembers=dailyN}
		if factorN<out.MinFactorMembers{out.MinFactorMembers=factorN}
		for _,api:=range []string{"daily","adj_factor"}{
			info,err:=os.Lstat(filepath.Join(c.dir,api+"_"+date+"_v1.json"))
			if err!=nil||!info.Mode().IsRegular(){return out,errors.New("invalid prewarm cache snapshot file")}
			if api=="daily"{out.DailySnapshots++;out.DailySnapshotBytes+=info.Size()} else {
				out.FactorSnapshots++;out.FactorSnapshotBytes+=info.Size()
			}
		}
		out.SessionsVerified++
	}
	if len(prepared.required)!=65{return out,errors.New("production prewarm did not include 65 required sessions")}
	for _,symbol:=range universe.Symbols {
		bars:=prepared.series[symbol]
		if len(bars)<len(prepared.required) {out.GapOrShort65Symbols++;continue}
		bars=bars[len(bars)-len(prepared.required):]
		gap,missingFactor:=false,false
		for i,day:=range prepared.required {
			if bars[i].Date!=day{gap=true;break}
			if bars[i].Adj<=0{missingFactor=true}
		}
		if gap{out.GapOrShort65Symbols++;continue}
		out.Contiguous65Symbols++
		if missingFactor {out.MissingFactor65Symbols++} else {out.Contiguous65WithFactors++}
	}
	// This is a conservative readiness INDICATOR, not a promise that any
	// symbol will pass the actual deterministic strategy.
	out.ReadyForPoolReview = out.Contiguous65WithFactors*100>=out.MainBoardSymbols*80
	if !out.ReadyForPoolReview {
		out.Warnings=append(out.Warnings,"Fewer than 80% of frozen main-board symbols have 65 contiguous bars and adjustment factors; review missing data before a full scan")
	}
	out.Warnings=append(out.Warnings,
		"Full 90-session source coverage is checked, but historic TuShare adjustment revisions are not independently proven point-in-time",
		"No strategy evaluation or persisted production pool was performed")
	return out,nil
}
