package fundamental

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "regexp"
 "sort"
 "strings"
 "time"
)

// FinancialStatement is an observed income report, not an LLM estimate.
// All currency values retain the upstream (CNY yuan) units.
type FinancialStatement struct {
 Symbol string `json:"symbol"`
 AnnouncementDate string `json:"announcement_date"`
 PeriodEnd string `json:"period_end"`
 Revenue *float64 `json:"revenue,omitempty"`
 NetIncome *float64 `json:"net_income,omitempty"`
 Source string `json:"source"`
}

type IncomeSource struct {
 Token string
 Endpoint string
 Client *http.Client
}
var symbolPattern=regexp.MustCompile(`^[0-9]{6}\.(SH|SZ|BJ)package fundamental

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "regexp"
 "sort"
 "strings"
 "time"
)

// FinancialStatement is an observed income report, not an LLM estimate.
// All currency values retain the upstream (CNY yuan) units.
type FinancialStatement struct {
 Symbol string `json:"symbol"`
 AnnouncementDate string `json:"announcement_date"`
 PeriodEnd string `json:"period_end"`
 Revenue *float64 `json:"revenue,omitempty"`
 NetIncome *float64 `json:"net_income,omitempty"`
 Source string `json:"source"`
}

type IncomeSource struct {
 Token string
 Endpoint string
 Client *http.Client
}
)
func dateOK(s string) bool { t,e:=time.Parse("20060102",s);return e==nil && t.Format("20060102")==s }

// LatestIncome returns the latest report actually announced by cutoff; it does
// not equate reporting period end with public availability.
func (s *IncomeSource) LatestIncome(ctx context.Context,symbol,cutoff string)(*FinancialStatement,error){
 if !symbolPattern.MatchString(symbol) {return nil,errors.New("invalid stock symbol")}
 if !dateOK(cutoff) {return nil,errors.New("invalid research cutoff")}
 if s==nil||strings.TrimSpace(s.Token)=="" {return nil,errors.New("tushare token is required")}
 endpoint:=s.Endpoint;if endpoint==""{endpoint="https://api.tushare.pro"}
 client:=s.Client;if client==nil{client=&http.Client{Timeout:20*time.Second}}
 reqBytes,err:=json.Marshal(map[string]any{"api_name":"income","token":s.Token,"params":map[string]string{"ts_code":symbol,"start_date":"19900101","end_date":cutoff},"fields":"ts_code,ann_date,end_date,revenue,n_income"})
 if err!=nil{return nil,err}
 req,err:=http.NewRequestWithContext(ctx,http.MethodPost,endpoint,bytes.NewReader(reqBytes));if err!=nil{return nil,err}
 req.Header.Set("Content-Type","application/json")
 res,err:=client.Do(req);if err!=nil{return nil,fmt.Errorf("tushare income: %w",err)}
 defer res.Body.Close()
 if res.StatusCode!=http.StatusOK{return nil,fmt.Errorf("tushare income HTTP %d",res.StatusCode)}
 raw,err:=io.ReadAll(io.LimitReader(res.Body,8<<20));if err!=nil{return nil,err}
 var output struct {Code int `json:"code"`;Msg string `json:"msg"`;Data struct{Fields []string `json:"fields"`;Items [][]json.RawMessage `json:"items"`} `json:"data"`}
 if err:=json.Unmarshal(raw,&output);err!=nil{return nil,fmt.Errorf("decode income: %w",err)}
 if output.Code!=0{return nil,fmt.Errorf("tushare income error code %d: %s",output.Code,output.Msg)}
 lookup:=map[string]int{};for i,name:=range output.Data.Fields{lookup[name]=i}
 for _,name:=range []string{"ts_code","ann_date","end_date","revenue","n_income"}{if _,ok:=lookup[name];!ok{return nil,fmt.Errorf("missing income field %s",name)}}
 var reports []FinancialStatement
 for _,row:=range output.Data.Items{
  if len(row)!=len(output.Data.Fields){return nil,errors.New("income row length does not match fields")}
  var code,ann,end string
  if err:=json.Unmarshal(row[lookup["ts_code"]],&code);err!=nil{return nil,err}
  if err:=json.Unmarshal(row[lookup["ann_date"]],&ann);err!=nil{return nil,err}
  if err:=json.Unmarshal(row[lookup["end_date"]],&end);err!=nil{return nil,err}
  if code!=symbol || !dateOK(ann)||!dateOK(end)||ann>cutoff||end>cutoff{continue}
  report:=FinancialStatement{Symbol:code,AnnouncementDate:ann,PeriodEnd:end,Source:"TUSHARE_INCOME"}
  if string(row[lookup["revenue"]])!="null"{if err:=json.Unmarshal(row[lookup["revenue"]],&report.Revenue);err!=nil{return nil,err}}
  if string(row[lookup["n_income"]])!="null"{if err:=json.Unmarshal(row[lookup["n_income"]],&report.NetIncome);err!=nil{return nil,err}}
  if report.Revenue==nil&&report.NetIncome==nil{continue}
  reports=append(reports,report)
 }
 if len(reports)==0{return nil,errors.New("no announced income statement available at requested cutoff")}
 sort.Slice(reports,func(i,j int)bool{if reports[i].PeriodEnd==reports[j].PeriodEnd{return reports[i].AnnouncementDate>reports[j].AnnouncementDate};return reports[i].PeriodEnd>reports[j].PeriodEnd})
 return &reports[0],nil
}
