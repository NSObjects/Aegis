package researchlang

import ("context";"errors";"testing")

func TestCheckChineseNarrativesOnlyWhenEnabled(t *testing.T){
 ctx:=RequireChinese(context.Background())
 for _,tc:=range []struct{label,s string;wantError bool}{
  {"Chinese with technical abbreviations","行情承压，MA20 与 MACD 指标存在背离。",false},
  {"English paragraph","The portfolio lacks sufficient information to determine its exposure.",true},
  {"English in mixed paragraph","存在风险。The market conditions are uncertain and require caution.",true},
  {"code only","RISK_OFF MACD 600519.SH",false},
  {"English acronym inside Chinese","需继续观察 RSI14 和 OHLC 数据。",false},
  {"evidence identifier","DOC_123 event_x1",false},
  {"proper name","Bank of China",false},
  {"empty","",false},
 }{
  t.Run(tc.label,func(t *testing.T){
   err:=Check(ctx,Field{Name:"summary",Text:tc.s})
   if (err!=nil)!=tc.wantError{t.Fatalf("Check(%q): %v",tc.s,err)}
   if err!=nil && !errors.Is(err,ErrLanguage){t.Fatalf("unexpected error kind %v",err)}
  })
 }
 if err:=Check(context.Background(),Field{Name:"summary",Text:"The portfolio lacks sufficient information to determine its exposure."});err!=nil {
  t.Fatal("legacy standalone callers must not change without opt-in")
 }
}
func TestLanguageErrorDoesNotEchoSensitiveText(t *testing.T){
 err:=Check(RequireChinese(context.Background()),Field{Name:"synthesis.summary",Text:"The private account valuation cannot be assessed without more evidence."})
 if err==nil || err.Error()=="The private account valuation cannot be assessed without more evidence."{t.Fatalf("unexpected output %v",err)}
}
