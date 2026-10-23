package main

import (
 "context"
 "strings"
 "testing"
)

func TestParseSymbols(t *testing.T) {
 values,err:=parseSymbols(" 600519.SH,000001.SZ ")
 if err!=nil||len(values)!=2||values[0]!="600519.SH"||values[1]!="000001.SZ"{t.Fatalf("values=%v err=%v",values,err)}
 for _,input:=range []string{"","600519.SH,600519.SH","600abc.SH","300001.SZ,","600519.BJ",strings.Repeat("600519.SH,",20)+"000001.SZ"}{
  if _,err:=parseSymbols(input);err==nil{t.Fatalf("accepted bad input %q",input)}
 }
}
func TestRunRequiresValidInputsBeforeProvider(t *testing.T){
 t.Setenv("TUSHARE_TOKEN","")
 if err:=run(context.Background(),"20261023","600519.SH");err==nil||!strings.Contains(err.Error(),"TUSHARE_TOKEN"){t.Fatalf("missing token: %v",err)}
 if err:=run(context.Background(),"20261388","600519.SH");err==nil{t.Fatal("invalid date accepted")}
 if err:=run(context.Background(),"20261023","bad");err==nil{t.Fatal("invalid stock accepted")}
}

func TestRunUniverseRejectsInvalidBoundsBeforeProvider(t *testing.T){
 t.Setenv("TUSHARE_TOKEN","")
 for _,tc:=range []struct{offset,limit int}{{-1,10},{0,0},{0,21}}{
  if err:=runUniverse(context.Background(),"20261022",tc.offset,tc.limit);err==nil||!strings.Contains(err.Error(),"batch size"){t.Fatalf("invalid bounds %+v: %v",tc,err)}
 }
 if err:=runUniverse(context.Background(),"20261022",0,20);err==nil||!strings.Contains(err.Error(),"TUSHARE_TOKEN"){t.Fatalf("missing credentials: %v",err)}
}
