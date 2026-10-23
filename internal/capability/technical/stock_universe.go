package technical

import (
 "context"
 "errors"
 "fmt"
 "sort"
 "strings"
 "time"
)

// StockUniverse is a reproducible snapshot of securities listed as of a given
// date, sourced from Tushare stock_basic. It does not imply all shares traded.
type StockUniverse struct {
 AsOf string `json:"as_of"`
 Source string `json:"source"`
 Symbols []string `json:"symbols"`
}

// ListStockUniverse checks all Tushare listing-status partitions so historic
// scans do not silently exclude currently delisted securities.
func (c *TushareClient) ListStockUniverse(ctx context.Context, asOf string)(StockUniverse,error) {
 out:=StockUniverse{AsOf:asOf,Source:"TUSHARE_STOCK_BASIC_L_D_P"}
 d,err:=time.Parse("20060102",asOf)
 if err!=nil||d.Format("20060102")!=asOf{return out,errors.New("invalid universe as-of")}
 if c==nil{return out,errors.New("nil Tushare client")}
 symbols:=map[string]bool{}
 for _,status:=range []string{"L","D","P"} {
  rows,err:=c.query(ctx,"stock_basic",map[string]any{"exchange":"","list_status":status},"ts_code","list_date","delist_date","list_status")
  if err!=nil{return out,fmt.Errorf("stock universe %s: %w",status,err)}
  // A valid empty listing-status partition is not a transport failure.
  for _,row:=range rows {
   symbol:=str(row["ts_code"])
   listed:=str(row["list_date"]);delisted:=str(row["delist_date"])
   if len(symbol)!=9||!(strings.HasSuffix(symbol,".SH")||strings.HasSuffix(symbol,".SZ")){continue}
   valid:=true;for i:=0;i<6;i++{if symbol[i]<'0'||symbol[i]>'9'{valid=false}}
   if !valid{continue}
   if !(strings.HasPrefix(symbol,"600")||strings.HasPrefix(symbol,"601")||strings.HasPrefix(symbol,"603")||strings.HasPrefix(symbol,"605")||strings.HasPrefix(symbol,"000")||strings.HasPrefix(symbol,"001")||strings.HasPrefix(symbol,"002")||strings.HasPrefix(symbol,"003")){continue}
   if len(listed)!=8||listed>asOf{continue}
   if delisted!=""&&delisted<=asOf{continue}
   symbols[symbol]=true
  }
 }
 for symbol:=range symbols{out.Symbols=append(out.Symbols,symbol)}
 sort.Strings(out.Symbols)
 if len(out.Symbols)==0{return out,errors.New("empty eligible stock universe")}
 return out,nil
}
