package technical

import (
 "context"
 "errors"
 "fmt"
 "math"
 "sort"
 "strconv"
 "strings"
 "time"
)

const requiredBars=120
type HTTPResearcher struct{ api *TushareClient }
func NewHTTPResearcher(token string)(*HTTPResearcher,error){c,e:=NewTushareClient(token);if e!=nil{return nil,e};return &HTTPResearcher{api:c},nil}
type bar struct{ Date string; Open,High,Low,Close,PreClose,Pct,Vol,Amount,Adj float64 }

func (r *HTTPResearcher) Research(ctx context.Context,req Request)(map[string]any,error){
 if r==nil||r.api==nil{return nil,errors.New("technical HTTP researcher is nil")}
 symbol:=strings.ToUpper(strings.TrimSpace(req.Symbol));asof:=strings.TrimSpace(req.AsOf);if symbol==""||asof==""{return nil,errors.New("technical research symbol and as_of are required")}
 d,err:=time.Parse("20060102",asof);if err!=nil{return nil,fmt.Errorf("invalid technical as_of %q: %w",asof,err)}
 basic,err:=r.api.query(ctx,"stock_basic",map[string]any{"ts_code":symbol},"ts_code","list_status","list_date","delist_date");if err!=nil{return nil,err}
 if len(basic)>0 {ld:=str(basic[0]["list_date"]);dd:=str(basic[0]["delist_date"]);st:=str(basic[0]["list_status"]);if ld!=""&&asof<ld{return statusMap(symbol,asof,"NOT_LISTED","", "requested date precedes listing date"),nil};if (dd!=""&&asof>dd)||(st=="D"&&(dd==""||asof>dd)){return statusMap(symbol,asof,"DELISTED","", "listing metadata indicates delisted"),nil}}
 cal,err:=r.api.query(ctx,"trade_cal",map[string]any{"exchange":exchange(symbol),"start_date":asof,"end_date":asof},"is_open","pretrade_date");if err!=nil{return nil,err};if len(cal)==0{return statusMap(symbol,asof,"NO_DATA","","trade calendar unavailable"),nil}
 effective:=asof;if str(cal[0]["is_open"])!="1"{effective=str(cal[0]["pretrade_date"])};if effective==""{return statusMap(symbol,asof,"NO_DATA","","cannot resolve effective trading date"),nil}
 target,err:=r.api.query(ctx,"daily",map[string]any{"ts_code":symbol,"trade_date":effective},"ts_code","trade_date");if err!=nil{return nil,err}
 fallback:=""
 if len(target)==0 {
  last,lastErr:=r.lastTrade(ctx,symbol,asof,d.AddDate(0,0,-120).Format("20060102"));if lastErr!=nil{return nil,lastErr}
  if last=="" {return statusMap(symbol,asof,"NO_DATA","","no daily bar is available at or before requested_as_of; absence is not sufficient evidence of suspension"),nil}
  effective=last
  fallback=fmt.Sprintf("requested trading-day bar %s is unavailable; using latest available bar %s at or before requested_as_of; absence is not sufficient evidence of suspension",asof,last)
 }
 start:=d.AddDate(0,0,-420).Format("20060102")
 daily,err:=r.api.query(ctx,"daily",map[string]any{"ts_code":symbol,"start_date":start,"end_date":effective},"ts_code","trade_date","open","high","low","close","pre_close","pct_chg","vol","amount");if err!=nil{return nil,err}
 adj,err:=r.api.query(ctx,"adj_factor",map[string]any{"ts_code":symbol,"start_date":start,"end_date":effective},"trade_date","adj_factor");if err!=nil{return nil,err}
 factors:=map[string]float64{};for _,x:=range adj{factors[str(x["trade_date"])]=num(x["adj_factor"])}
 bars:=make([]bar,0,len(daily));for _,x:=range daily{date:=str(x["trade_date"]);af,ok:=factors[date];if !ok||af==0{continue};bars=append(bars,bar{Date:date,Open:num(x["open"]),High:num(x["high"]),Low:num(x["low"]),Close:num(x["close"]),PreClose:num(x["pre_close"]),Pct:num(x["pct_chg"]),Vol:num(x["vol"])*100,Amount:num(x["amount"])*1000,Adj:af})}
 sort.Slice(bars,func(i,j int)bool{return bars[i].Date<bars[j].Date});if len(bars)==0{return statusMap(symbol,asof,"NO_DATA","","daily and adjustment factor series do not overlap"),nil}
 history,historyErr:=observedHistoricalBars(bars)
 latest:=bars[len(bars)-1];anchor:=latest.Adj;q:=make([]float64,len(bars));for i,b:=range bars{q[i]=b.Close*b.Adj/anchor}
 db,_:=r.api.query(ctx,"daily_basic",map[string]any{"ts_code":symbol,"trade_date":latest.Date},"turnover_rate");var turnover any=nil;if len(db)>0{turnover=num(db[0]["turnover_rate"])}
 warns:=[]string{};if fallback!=""{warns=append(warns,fallback)};if historyErr!=nil{warns=append(warns,"historical_ohlcv_unavailable: invalid or duplicate observed trading dates");history=nil};if len(bars)<requiredBars{warns=append(warns,fmt.Sprintf("insufficient_history: %d trading bars available; %d required",len(bars),requiredBars))}
 return map[string]any{"symbol":symbol,"requested_as_of":asof,"effective_as_of":latest.Date,"last_trade_date":latest.Date,"status":"NORMAL",
  "price_basis":map[string]any{"snapshot":"RAW","technical":"PIT_QFQ","pit_anchor":latest.Date,"anchor_adj_factor":round(anchor,6)},
  "historical_ohlcv":map[string]any{"version":1,"price_basis":"RAW","bars":history,"pit_adjustment_factors":historicalPITFactors(bars)},
  "snapshot":map[string]any{"open":latest.Open,"high":latest.High,"low":latest.Low,"close":latest.Close,"pre_close":latest.PreClose,"change_percent":latest.Pct,"volume":latest.Vol,"amount":latest.Amount,"turnover_rate":turnover},
  "returns":map[string]any{"return_1d":ret(q,1),"return_5d":ret(q,5),"return_10d":ret(q,10),"return_20d":ret(q,20),"return_60d":ret(q,60)},
  "moving_averages":map[string]any{"ma5":ma(q,5),"ma10":ma(q,10),"ma20":ma(q,20),"ma60":ma(q,60)},
  "momentum":map[string]any{"rsi14":rsi(q,14),"macd":macd(q)},
  "volume":volumeMetrics(bars),"coverage":map[string]any{"bars":len(bars),"required_bars":requiredBars,"sufficient_history":len(bars)>=requiredBars,"start_trade_date":bars[0].Date,"end_trade_date":latest.Date},"warnings":warns},nil
}
func (r *HTTPResearcher) lastTrade(ctx context.Context,symbol,end,start string)(string,error){x,e:=r.api.query(ctx,"daily",map[string]any{"ts_code":symbol,"start_date":start,"end_date":end},"trade_date");if e!=nil{return "",e};last:="";for _,v:=range x{if d:=str(v["trade_date"]);d>last{last=d}};return last,nil}
func statusMap(s,a,st,last,w string)map[string]any{return map[string]any{"symbol":s,"requested_as_of":a,"effective_as_of":"","last_trade_date":last,"status":st,"price_basis":map[string]any{"snapshot":"RAW","technical":"PIT_QFQ","pit_anchor":""},"snapshot":nil,"returns":map[string]any{},"moving_averages":map[string]any{},"momentum":map[string]any{"rsi14":nil,"macd":map[string]any{"dif":nil,"dea":nil,"hist":nil}},"volume":map[string]any{},"coverage":map[string]any{"bars":0,"required_bars":requiredBars,"sufficient_history":false},"warnings":[]string{w}}}
func exchange(s string)string{if strings.HasSuffix(s,".SH"){return "SSE"};if strings.HasSuffix(s,".SZ"){return "SZSE"};if strings.HasSuffix(s,".BJ"){return "BSE"};return ""}
func str(v any)string{if v==nil{return ""};return strings.TrimSpace(fmt.Sprint(v))}
func num(v any)float64{switch x:=v.(type){case float64:return x;case string:n,_:=strconv.ParseFloat(x,64);return n};return 0}
func round(v float64,n int)float64{p:=math.Pow10(n);return math.Round(v*p)/p}
func ret(x []float64,n int)any{if len(x)<=n||x[len(x)-1-n]==0{return nil};return round((x[len(x)-1]/x[len(x)-1-n]-1)*100,4)}
func ma(x []float64,n int)any{if len(x)<n{return nil};s:=0.0;for _,v:=range x[len(x)-n:]{s+=v};return round(s/float64(n),4)}
func ema(x []float64,n int)[]float64{o:=make([]float64,len(x));if len(x)==0{return o};o[0]=x[0];k:=2/float64(n+1);for i:=1;i<len(x);i++{o[i]=x[i]*k+o[i-1]*(1-k)};return o}
func macd(x []float64)map[string]any{if len(x)==0{return map[string]any{"dif":nil,"dea":nil,"hist":nil}};a,b:=ema(x,12),ema(x,26);dif:=make([]float64,len(x));for i:=range x{dif[i]=a[i]-b[i]};dea:=ema(dif,9);i:=len(x)-1;return map[string]any{"dif":round(dif[i],4),"dea":round(dea[i],4),"hist":round(2*(dif[i]-dea[i]),4)}}
func rsi(x []float64,n int)any{if len(x)<=n{return nil};g,l:=0.0,0.0;for i:=1;i<=n;i++{d:=x[i]-x[i-1];if d>0{g+=d}else{l-=d}};g/=float64(n);l/=float64(n);for i:=n+1;i<len(x);i++{d:=x[i]-x[i-1];up,dn:=0.0,0.0;if d>0{up=d}else{dn=-d};g=(g*float64(n-1)+up)/float64(n);l=(l*float64(n-1)+dn)/float64(n)};if l==0{return 100.0};return round(100-100/(1+g/l),4)}
func volumeMetrics(b []bar)map[string]any{vals:=func(n int,amount bool)any{if len(b)<n{return nil};s:=0.0;for _,x:=range b[len(b)-n:]{if amount{s+=x.Amount}else{s+=x.Vol}};return round(s/float64(n),4)};vm5,vm20:=vals(5,false),vals(20,false);am5,am20:=vals(5,true),vals(20,true);latest:=b[len(b)-1];ratio:=func(v float64,m any)any{x,ok:=m.(float64);if !ok||x==0{return nil};return round(v/x,4)};return map[string]any{"volume_ma5":vm5,"volume_ma20":vm20,"volume_vs_ma5":ratio(latest.Vol,vm5),"volume_vs_ma20":ratio(latest.Vol,vm20),"amount_ma5":am5,"amount_ma20":am20,"amount_vs_ma5":ratio(latest.Amount,am5),"amount_vs_ma20":ratio(latest.Amount,am20)}}

func historicalPITFactors(bars []bar) []PITAdjustmentFactor {
 factors:=make([]PITAdjustmentFactor,len(bars))
 for i,b:=range bars { factors[i]=PITAdjustmentFactor{TradeDate:b.Date,Factor:b.Adj} }
 return factors
}
