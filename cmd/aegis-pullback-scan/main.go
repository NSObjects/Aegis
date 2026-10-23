// Command aegis-pullback-scan runs the deterministic pullback scanner against
// existing Tushare Technical Capability. No LLM or order execution is used.
package main

import (
 "context"
 "encoding/json"
 "errors"
 "flag"
 "fmt"
 "os"
 "strings"
 "time"

 "Aegis/internal/capability/technical"
 "Aegis/internal/strategy/pullback"
)

func parseSymbols(raw string)([]string,error){
 if strings.TrimSpace(raw)==""{return nil,errors.New("provide at least one stock symbol")}
 values:=strings.Split(raw,",")
 if len(values)>20{return nil,errors.New("maximum 20 symbols per invocation")}
 seen:=map[string]bool{}
 for i,s:=range values {
  s=strings.TrimSpace(s)
  if len(s)!=9 || ( !strings.HasSuffix(s,".SH") && !strings.HasSuffix(s,".SZ")){return nil,fmt.Errorf("invalid symbol %q",s)}
  for j:=0;j<6;j++{if s[j]<'0'||s[j]>'9'{return nil,fmt.Errorf("invalid symbol %q",s)}}
  if seen[s]{return nil,fmt.Errorf("duplicate symbol %q",s)}
  seen[s]=true
  values[i]=s
 }
 return values,nil
}

func run(ctx context.Context,asOf,raw string)error{
 symbols,err:=parseSymbols(raw);if err!=nil{return err}
 if d,e:=time.Parse("20060102",asOf);e!=nil||d.Format("20060102")!=asOf{return errors.New("invalid --as-of")}
 token:=strings.TrimSpace(os.Getenv("TUSHARE_TOKEN"))
 if token==""{return errors.New("TUSHARE_TOKEN required")}
 researcher,err:=technical.NewHTTPResearcher(token);if err!=nil{return err}
 results,err:=pullback.ScanResearcher(ctx,researcher,asOf,symbols);if err!=nil{return err}
 return json.NewEncoder(os.Stdout).Encode(struct{
  Strategy string `json:"strategy_version"`
  AsOf string `json:"as_of"`
  Results []pullback.Result `json:"results"`
 }{pullback.StrategyVersion,asOf,results})
}
// runUniverse scans one explicitly bounded slice of a point-in-time listing
// universe. The caller controls page offset; full-market scans must orchestrate
// multiple bounded invocations rather than exhausting provider quotas at once.
func runUniverse(ctx context.Context,asOf string,offset,limit int)error {
 if offset<0||limit<1||limit>20{return errors.New("universe offset must be nonnegative and batch size 1..20")}
 token:=strings.TrimSpace(os.Getenv("TUSHARE_TOKEN"))
 if token==""{return errors.New("TUSHARE_TOKEN required")}
 client,err:=technical.NewTushareClient(token);if err!=nil{return err}
 universe,err:=client.ListStockUniverse(ctx,asOf);if err!=nil{return err}
 if offset>len(universe.Symbols){return errors.New("universe offset exceeds size")}
 end:=offset+limit;if end>len(universe.Symbols){end=len(universe.Symbols)}
 symbols:=universe.Symbols[offset:end]
 researcher,err:=technical.NewHTTPResearcher(token);if err!=nil{return err}
 results,err:=pullback.ScanResearcher(ctx,researcher,asOf,symbols);if err!=nil{return err}
 return json.NewEncoder(os.Stdout).Encode(struct{
  Strategy string `json:"strategy_version"`
  Universe technical.StockUniverse `json:"universe"`
  Offset int `json:"offset"`
  Limit int `json:"limit"`
  Results []pullback.Result `json:"results"`
 }{pullback.StrategyVersion,technical.StockUniverse{AsOf:universe.AsOf,Source:universe.Source,Symbols:nil},offset,limit,results})
}
func main(){
 asOf:=flag.String("as-of","","completed trading date YYYYMMDD")
 symbols:=flag.String("symbols","","comma-separated A-share codes (max 20)")
 timeout:=flag.Duration("timeout",3*time.Minute,"overall deadline")
 useUniverse:=flag.Bool("universe",false,"use point-in-time Tushare stock universe instead of --symbols")
 offset:=flag.Int("offset",0,"sorted universe slice offset")
 limit:=flag.Int("limit",20,"universe slice length, max 20")
 flag.Parse()
 ctx,cancel:=context.WithTimeout(context.Background(),*timeout);defer cancel()
 var err error
 if *useUniverse {
  if strings.TrimSpace(*symbols)!=""{fmt.Fprintln(os.Stderr,"--universe and --symbols are mutually exclusive");os.Exit(1)}
  err=runUniverse(ctx,*asOf,*offset,*limit)
 } else {
  err=run(ctx,*asOf,*symbols)
 }
 if err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
}
