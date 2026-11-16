// Command aegis-market-prewarm validates 90 TuShare market sessions using
// exactly the same date cache preflight as the production PIT main-board pool.
// This is an explicit operator canary; it NEVER starts stock scans or trades.
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

type prewarmPlan struct {
	AsOf             string `json:"as_of"`
	Sessions         int    `json:"sessions"`
	MaxProviderCalls int    `json:"max_provider_calls"`
	NetworkEnabled   bool   `json:"network_enabled"`
	Message          string `json:"message"`
}

func completedMarketDate(asOf string, now time.Time) bool {
	day,err:=time.Parse("20060102",asOf)
	if err!=nil||day.Format("20060102")!=asOf{return false}
	today:=now.In(time.FixedZone("Asia/Shanghai",8*3600)).Format("20060102")
	return asOf<today
}

func run(ctx context.Context, asOf string, execute bool, token string, stdout, stderr io.Writer, now time.Time) error {
	if ctx==nil||stdout==nil||stderr==nil{return errors.New("prewarm requires context and writers")}
	if !completedMarketDate(asOf,now){
		return errors.New("--as-of must be an already-completed Shanghai calendar date (YYYYMMDD) before today")
	}
	encoder:=json.NewEncoder(stdout)
	encoder.SetIndent("","  ")
	if !execute {
		return encoder.Encode(prewarmPlan{
			AsOf:asOf,Sessions:technical.MarketCacheSessionCount,
			MaxProviderCalls:4+2*technical.MarketCacheSessionCount,
			NetworkEnabled:false,
			Message:"PREVIEW ONLY: no real TuShare calls and no cache was prewarmed; --execute required for LIVE",
		})
	}
	if strings.TrimSpace(token)=="" {return errors.New("LIVE prewarm requires TUSHARE_TOKEN")}
	client,err:=technical.NewCanaryTushareClient(token)
	if err!=nil{return errors.New("invalid local TuShare token")}
	// A standalone disposable scratch cache ensures a LIVE acceptance run
	// can never clobber production snapshots or operator pool records.
	dir,err:=os.MkdirTemp("","aegis-market-full-prewarm-")
	if err!=nil{return fmt.Errorf("cannot create private scratch: %w",err)}
	defer os.RemoveAll(dir)
	cache,err:=technical.NewMarketDayCache(client,dir)
	if err!=nil{return fmt.Errorf("cannot create private scratch cache: %w",err)}
	progress:=func(done,total int)error{
		if done==1||done%10==0||done==total{
			_,_ = fmt.Fprintf(stderr,"90-session prewarm: %d/%d complete\n",done,total)
		}
		return ctx.Err()
	}
	report,err:=cache.ProbeFullPrewarm(ctx,asOf,progress)
	if err!=nil {
		return fmt.Errorf("LIVE production-cache prewarm rejected: %s",
			strings.ReplaceAll(err.Error(),strings.TrimSpace(token),"[REDACTED]"))
	}
	if err:=encoder.Encode(report);err!=nil{return err}
	if !report.ReadyForPoolReview{
		return errors.New("LIVE prewarm data readiness failed; do not initiate full-pool scan")
	}
	return nil
}

func main() {
	asOf:=flag.String("as-of","","already completed Shanghai trading day YYYYMMDD")
	execute:=flag.Bool("execute",false,"deliberately allow up to 184 TuShare requests (default: zero-network preview)")
	timeout:=flag.Duration("timeout",12*time.Minute,"overall bound (3m..15m)")
	flag.Parse()
	if *timeout<3*time.Minute||*timeout>15*time.Minute{
		fmt.Fprintln(os.Stderr,"--timeout must be 3m..15m")
		os.Exit(2)
	}
	ctx,cancel:=context.WithTimeout(context.Background(),*timeout)
	defer cancel()
	token:=""
	if *execute{token=os.Getenv("TUSHARE_TOKEN")}
	if err:=run(ctx,*asOf,*execute,token,os.Stdout,os.Stderr,time.Now());err!=nil{
		fmt.Fprintln(os.Stderr,err)
		os.Exit(1)
	}
}
