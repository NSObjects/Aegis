package application

import (
 "errors"
 "testing"
)

func TestTechnicalMarketFailureCategories(t *testing.T) {
 cases:=[]struct{message,want string}{
  {"technical agent returned empty output","TECHNICAL_MODEL_EMPTY"},
  {"invalid technical context JSON: invalid character","TECHNICAL_JSON_DECODE"},
  {"invalid technical context: missing OHLC","TECHNICAL_CONTEXT_VALIDATION"},
  {"technical context symbol mismatch","TECHNICAL_CONTEXT_VALIDATION"},
  {"market agent returned empty output","MARKET_MODEL_EMPTY"},
  {"invalid market context JSON: invalid character","MARKET_JSON_DECODE"},
  {"invalid market context after one correction: missing as_of","MARKET_CONTEXT_VALIDATION"},
  {"unrelated provider failure SECRET_TOKEN=abcdef","UNKNOWN"},
  {"market agent run failed: opaque provider error","UNKNOWN"},
 }
 for _,tc:=range cases{
  t.Run(tc.want+"_"+tc.message,func(t *testing.T){
   if got:=researchErrorCategory(errors.New(tc.message));got!=tc.want{t.Fatalf("got %q want %q",got,tc.want)}
  })
 }
}
