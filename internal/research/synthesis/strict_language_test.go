package synthesis

import (
 "context"
 "errors"
 "testing"
 "Aegis/internal/researchlang"
 "github.com/cloudwego/eino/schema"
)

func TestSynthesisRetriesEnglishAndAcceptsVerifiedChinese(t *testing.T){
 m:=&synthesisModel{responses:[]*schema.AgenticMessage{
  synthesisAssistantMessage(`{"summary":"The market remains uncertain and requires further financial research.","confidence":0.6}`),
  synthesisAssistantMessage(`{"summary":"市场尚无明确方向，需要继续核实基本信息。","confidence":0.6}`),
 }}
 res,err:=(&AgenticSynthesizer{Model:m}).Synthesize(researchlang.RequireChinese(context.Background()),validSynthesisInput())
 if err!=nil{t.Fatal(err)}
 if m.calls!=2 || res.Synthesis.Summary!="市场尚无明确方向，需要继续核实基本信息。"{t.Fatalf("calls=%d summary=%q",m.calls,res.Synthesis.Summary)}
}
func TestSynthesisRejectsRepeatedEnglishWithoutDroppingClaims(t *testing.T){
 m:=&synthesisModel{response:synthesisAssistantMessage(`{"summary":"The market remains uncertain and requires further financial research.","confidence":0.6}`)}
 res,err:=(&AgenticSynthesizer{Model:m}).Synthesize(researchlang.RequireChinese(context.Background()),validSynthesisInput())
 if res!=nil||!errors.Is(err,researchlang.ErrLanguage)||m.calls!=2{t.Fatalf("res=%+v err=%v calls=%d",res,err,m.calls)}
}
