package technical

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "math"
 "os"
 "path/filepath"
 "sort"
 "strconv"
 "strings"
 "sync"
 "time"
)

const (
 marketDaySnapshotVersion = 1
 marketDayMaxRows = 6000
 marketDaySessions = 90
 MarketCacheSessionCount = marketDaySessions
 marketDayCalendarLookback = 240
)

// MarketDayCache persists validated TuShare single-date cross-sections.
// A response at the documented 6000-row cap is NOT proof of completeness,
// so it is rejected rather than persisted. Files contain no provider token.
type MarketDayCache struct {
 api *TushareClient
 dir string
 mu sync.Mutex
 // Deliberately below typical TuShare per-minute quotas. Cache hits are free.
 requestInterval time.Duration
}

type marketPriceRow struct {
 Symbol string `json:"symbol"`
 Date string `json:"date"`
 Open float64 `json:"open"`
 High float64 `json:"high"`
 Low float64 `json:"low"`
 Close float64 `json:"close"`
 Volume float64 `json:"volume"`
 Amount float64 `json:"amount"`
}
type marketFactorRow struct {
 Symbol string `json:"symbol"`
 Date string `json:"date"`
 Factor float64 `json:"factor"`
}
type marketDayBody struct {
 Version int `json:"version"`
 API string `json:"api"`
 Date string `json:"date"`
 Prices []marketPriceRow `json:"prices,omitempty"`
 Factors []marketFactorRow `json:"factors,omitempty"`
}
type marketDayFile struct {
 marketDayBody
 SHA256 string `json:"sha256"`
}

func NewMarketDayCache(client *TushareClient, dir string) (*MarketDayCache,error) {
 if client==nil || strings.TrimSpace(dir)=="" {return nil,errors.New("market day cache requires client and directory")}
 if err:=os.MkdirAll(dir,0700);err!=nil{return nil,err}
 info,err:=os.Lstat(dir)
 if err!=nil || !info.IsDir() || info.Mode().Perm()&0077!=0 {return nil,errors.New("market day cache directory must be private")}
 return &MarketDayCache{api:client,dir:dir,requestInterval:400*time.Millisecond},nil
}
func marketSnapshotHash(body marketDayBody) (string,error) {
 raw,err:=json.Marshal(body);if err!=nil{return "",err}
 sum:=sha256.Sum256(raw)
 return hex.EncodeToString(sum[:]),nil
}
func marketSymbol(s string) bool {
 if len(s)!=9 || !(strings.HasSuffix(s,".SH")||strings.HasSuffix(s,".SZ")||strings.HasSuffix(s,".BJ")) {return false}
 for i:=0;i<6;i++ {if s[i]<'0'||s[i]>'9'{return false}}
 return true
}
func strictMarketNumber(v any)(float64,error) {
 var n float64
 switch t:=v.(type) {
 case float64:n=t
 case json.Number:
  x,e:=t.Float64();if e!=nil{return 0,e};n=x
 case string:
  if strings.TrimSpace(t)=="" {return 0,errors.New("empty provider number")}
  x,e:=strconv.ParseFloat(t,64);if e!=nil{return 0,e};n=x
 default:return 0,fmt.Errorf("invalid provider number type %T",v)
 }
 if math.IsNaN(n)||math.IsInf(n,0){return 0,errors.New("non-finite provider number")}
 return n,nil
}
func validMarketBody(b marketDayBody) error {
 if b.Version!=marketDaySnapshotVersion || (b.API!="daily"&&b.API!="adj_factor") ||
  !withinPoolWindow(b.Date,b.Date,b.Date) {return errors.New("invalid market day identity")}
 var last string
 if b.API=="daily" {
  if len(b.Factors)!=0 || len(b.Prices)==0 || len(b.Prices)>=marketDayMaxRows {return errors.New("market daily snapshot empty or capped")}
  for _,p:=range b.Prices {
   if !marketSymbol(p.Symbol)||p.Date!=b.Date||p.Symbol<=last {return errors.New("invalid or duplicate market daily symbol/date")}
   last=p.Symbol
   for _,x:=range []float64{p.Open,p.High,p.Low,p.Close,p.Volume,p.Amount}{
    if x<0||math.IsNaN(x)||math.IsInf(x,0){return errors.New("invalid market price")}
   }
   if p.Open<=0||p.Low<=0||p.Close<=0||p.High<p.Low||
    p.High<math.Max(p.Open,p.Close)||p.Low>math.Min(p.Open,p.Close) {
    return errors.New("inconsistent market OHLC")
   }
  }
 } else {
  if len(b.Prices)!=0 || len(b.Factors)==0 || len(b.Factors)>=marketDayMaxRows {return errors.New("market factor snapshot empty or capped")}
  for _,f:=range b.Factors {
   if !marketSymbol(f.Symbol)||f.Date!=b.Date||f.Symbol<=last||
    f.Factor<=0||math.IsNaN(f.Factor)||math.IsInf(f.Factor,0) {return errors.New("invalid or duplicate market factor")}
   last=f.Symbol
  }
 }
 return nil
}
func (c *MarketDayCache) readDate(ctx context.Context, api,date string)(marketDayBody,error) {
 var empty marketDayBody
 if c==nil || (api!="daily"&&api!="adj_factor") || !withinPoolWindow(date,date,date) {
  return empty,errors.New("invalid market day cache request")
 }
 // The pool runner is serial. Holding the lock also prevents duplicate
 // writes if another reader is added later.
 c.mu.Lock()
 defer c.mu.Unlock()
 path:=filepath.Join(c.dir,api+"_"+date+"_v1.json")
 info,err:=os.Lstat(path)
 if err==nil {
  if !info.Mode().IsRegular()||info.Mode().Perm()&0077!=0||info.Size()>(8<<20) {
   return empty,errors.New("market day cache file is unsafe")
  }
  raw,err:=os.ReadFile(path)
  if err!=nil{return empty,err}
  dec:=json.NewDecoder(strings.NewReader(string(raw)))
  dec.DisallowUnknownFields()
  var saved marketDayFile
  if err:=dec.Decode(&saved);err!=nil{return empty,fmt.Errorf("corrupt market day cache: %w",err)}
  var tail any
  if !errors.Is(dec.Decode(&tail),io.EOF) {
   return empty,errors.New("market day cache contains trailing data")
  }
  if saved.API!=api || saved.Date!=date {return empty,errors.New("market day cache identity mismatch")}
  if err:=validMarketBody(saved.marketDayBody);err!=nil{return empty,err}
  want,err:=marketSnapshotHash(saved.marketDayBody)
  if err!=nil{return empty,err}
  if saved.SHA256!=want{return empty,errors.New("market day cache checksum mismatch")}
  return saved.marketDayBody,nil
 }
 if !os.IsNotExist(err){return empty,err}
 if c.requestInterval>0 {
  timer:=time.NewTimer(c.requestInterval)
  select {
  case <-ctx.Done():timer.Stop();return empty,ctx.Err()
  case <-timer.C:
  }
 }
 fields:=[]string{"ts_code","trade_date","open","high","low","close","vol","amount"}
 if api=="adj_factor" {fields=[]string{"ts_code","trade_date","adj_factor"}}
 rows,err:=c.api.query(ctx,api,map[string]any{"trade_date":date},fields...)
 if err!=nil{return empty,err}
 // Even an apparently valid partial response must never be cached. TuShare
 // documents a 6000-row cap, including for date-partitioned queries.
 if len(rows)==0||len(rows)>=marketDayMaxRows {
  return empty,fmt.Errorf("market day %s/%s returned %d rows (empty or provider cap)",api,date,len(rows))
 }
 b:=marketDayBody{Version:marketDaySnapshotVersion,API:api,Date:date}
 for _,row:=range rows {
  symbol,tradeDate:=str(row["ts_code"]),str(row["trade_date"])
  if api=="adj_factor" {
   factor,err:=strictMarketNumber(row["adj_factor"]);if err!=nil{return empty,err}
   b.Factors=append(b.Factors,marketFactorRow{Symbol:symbol,Date:tradeDate,Factor:factor})
  } else {
   vals:=make([]float64,0,6)
   for _,key:=range []string{"open","high","low","close","vol","amount"} {
    value,err:=strictMarketNumber(row[key]);if err!=nil{return empty,err}
    vals=append(vals,value)
   }
   b.Prices=append(b.Prices,marketPriceRow{
    Symbol:symbol,Date:tradeDate,Open:vals[0],High:vals[1],Low:vals[2],Close:vals[3],
    Volume:vals[4]*100,Amount:vals[5]*1000,
   })
  }
 }
 sort.Slice(b.Prices,func(i,j int)bool{return b.Prices[i].Symbol<b.Prices[j].Symbol})
 sort.Slice(b.Factors,func(i,j int)bool{return b.Factors[i].Symbol<b.Factors[j].Symbol})
 if err:=validMarketBody(b);err!=nil{return empty,err}
 digest,err:=marketSnapshotHash(b);if err!=nil{return empty,err}
 raw,err:=json.Marshal(marketDayFile{marketDayBody:b,SHA256:digest});if err!=nil{return empty,err}
 tmp,err:=os.CreateTemp(c.dir,".market-day-*.tmp");if err!=nil{return empty,err}
 defer os.Remove(tmp.Name())
 if err=tmp.Chmod(0600);err==nil {_,err=tmp.Write(raw)}
 if err==nil {err=tmp.Sync()}
 closeErr:=tmp.Close()
 if err!=nil{return empty,err}
 if closeErr!=nil{return empty,closeErr}
 if err:=os.Rename(tmp.Name(),path);err!=nil{return empty,err}
 return b,nil
}

// tradingSessions obtains and checks the complete contiguous calendar range.
// A missing session cannot silently shrink MA60/MA20 windows.
func (c *MarketDayCache) tradingSessions(ctx context.Context, asOf string)([]string,error) {
 if !withinPoolWindow(asOf,asOf,asOf){return nil,errors.New("invalid pool cache cutoff")}
 end,_:=time.Parse("20060102",asOf)
 begin:=end.AddDate(0,0,-marketDayCalendarLookback)
 from:=begin.Format("20060102")
 rows,err:=c.api.query(ctx,"trade_cal",map[string]any{
  "exchange":"SSE","start_date":from,"end_date":asOf,
 },"cal_date","is_open")
 if err!=nil{return nil,err}
 byDate:=make(map[string]bool,len(rows))
 for _,row:=range rows {
  date:=str(row["cal_date"])
  if !withinPoolWindow(date,from,asOf) {return nil,errors.New("market calendar out-of-range date")}
  if _,ok:=byDate[date];ok{return nil,errors.New("duplicate market calendar date")}
  v:=str(row["is_open"])
  if v!="0"&&v!="1" {return nil,errors.New("invalid market calendar trading flag")}
  byDate[date]=v=="1"
 }
 sessions:=[]string{}
 for d:=begin;!d.After(end);d=d.AddDate(0,0,1) {
  date:=d.Format("20060102")
  open,exists:=byDate[date]
  if !exists{return nil,fmt.Errorf("incomplete market trading calendar at %s",date)}
  if open{sessions=append(sessions,date)}
 }
 if len(sessions)<marketDaySessions||sessions[len(sessions)-1]!=asOf {
  return nil,errors.New("market calendar has insufficient sessions or non-trading cutoff")
 }
 return sessions[len(sessions)-marketDaySessions:],nil
}

type marketPrepared struct {
 asOf string
 series map[string][]bar
 required []string
}
func (p *marketPrepared) research(symbol, asOf string)(map[string]any,error) {
 if p==nil||p.asOf!=asOf {return nil,errors.New("market day cache not prepared for requested cutoff")}
 bars,exists:=p.series[symbol]
 if !exists{return nil,errors.New("symbol outside frozen market universe")}
 if len(bars)==0 {
  return statusMap(symbol,asOf,"NO_DATA","","no observed daily bars in market cache"),nil
 }
 // A daily row missing within the final 65 market sessions might be a
 // suspension or a partial provider response. Never infer that the symbol
 // traded: mark it insufficient rather than selecting from a shifted window.
 if len(bars)<len(p.required) {
  return statusMap(symbol,asOf,"NO_DATA","","gaps in last 65 market sessions"),nil
 }
 last:=bars[len(bars)-len(p.required):]
 for i,day:=range p.required {
  if last[i].Date!=day {
   return statusMap(symbol,asOf,"NO_DATA","","gaps in last 65 market sessions"),nil
  }
 }
 history,err:=observedHistoricalBars(last)
 if err!=nil{return nil,err}
 raw:=map[string]any{"version":1,"price_basis":"RAW","bars":history}
 factors:=make([]PITAdjustmentFactor,0,len(last))
 for _,candle:=range last {
  if candle.Adj<=0 {factors=nil;break}
  factors=append(factors,PITAdjustmentFactor{TradeDate:candle.Date,Factor:candle.Adj})
 }
 if len(factors)==len(history) {raw["pit_adjustment_factors"]=factors}
 return map[string]any{
  "status":"NORMAL","symbol":symbol,
  "requested_as_of":asOf,"effective_as_of":asOf,
  "historical_ohlcv":raw,
 },nil
}

// Prepare constructs a bounded in-memory index of exactly the frozen universe.
// A failed/missing/capped date leaves no usable new index; completed date
// snapshots remain on disk so manual resumption only fetches missing days.
func (c *MarketDayCache) Prepare(ctx context.Context, asOf string, symbols []string, progress func(done,total int)error)(*marketPrepared,error) {
 sessions,err:=c.tradingSessions(ctx,asOf)
 if err!=nil{return nil,err}
 membership:=make(map[string]struct{},len(symbols))
 series:=make(map[string][]bar,len(symbols))
 for _,symbol:=range symbols {
  if !marketSymbol(symbol)||!IsMainBoardSymbol(symbol){return nil,errors.New("invalid frozen pool member")}
  if _,exists:=membership[symbol];exists{return nil,errors.New("duplicate frozen pool member")}
  membership[symbol]=struct{}{}
  series[symbol]=nil
 }
 for i,date:=range sessions {
  if err:=ctx.Err();err!=nil{return nil,err}
  prices,err:=c.readDate(ctx,"daily",date)
  if err!=nil{return nil,fmt.Errorf("market day daily %s: %w",date,err)}
  factors,err:=c.readDate(ctx,"adj_factor",date)
  if err!=nil{return nil,fmt.Errorf("market day factors %s: %w",date,err)}
  factorsBySymbol:=make(map[string]float64,len(factors.Factors))
  factorMembers:=0
  for _,row:=range factors.Factors{
   factorsBySymbol[row.Symbol]=row.Factor
   if _,ok:=membership[row.Symbol];ok{factorMembers++}
  }
  priceMembers:=0
  for _,row:=range prices.Prices{
   if _,ok:=membership[row.Symbol];!ok {continue}
   priceMembers++
   series[row.Symbol]=append(series[row.Symbol],bar{
    Date:row.Date,Open:row.Open,High:row.High,Low:row.Low,Close:row.Close,
    Vol:row.Volume,Amount:row.Amount,Adj:factorsBySymbol[row.Symbol],
   })
  }
  // A response well below the advertised cap may still be a transient,
  // incomplete provider page. Against a large frozen pool, reject severe
  // undercoverage rather than publishing thousands of "no data" results.
  // Thresholds are intentionally conservative: this is NOT a guarantee
  // of complete market coverage, especially around new listings/suspensions.
  if len(membership)>=100 &&
   (factorMembers*100<len(membership)*90 || priceMembers*100<len(membership)*80) {
    return nil,fmt.Errorf("market day %s has insufficient pool coverage: daily=%d factors=%d expected=%d",
     date,priceMembers,factorMembers,len(membership))
  }
  if progress!=nil {if err:=progress(i+1,len(sessions));err!=nil{return nil,err}}
 }
 return &marketPrepared{
  asOf:asOf,series:series,
  required:append([]string(nil),sessions[len(sessions)-65:]...),
 },nil
}
