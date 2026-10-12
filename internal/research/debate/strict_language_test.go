package debate

import (
 "context"
 "errors"
 "testing"
 "Aegis/internal/researchlang"
)

const englishBullTurn=`{"round":1,"side":"BULL","thesis":"The company presents an opportunity that needs further evaluation.","arguments":[{"id":"bull-r1-1","claim":"技术指标显示部分支撑。","references":[{"domain":"TECHNICAL"}]}],"confidence":0.7}`
const chineseBullTurn=`{"round":1,"side":"BULL","thesis":"技术证据存在潜在机会，但仍需要后续验证。","arguments":[{"id":"bull-r1-1","claim":"技术指标显示部分支撑。","references":[{"domain":"TECHNICAL"}]}],"confidence":0.7}`

func TestBullTurnChineseRepairPreservesGrounding(t *testing.T){
 m:=&scriptedTurnModel{outputs:[]string{englishBullTurn,chineseBullTurn}}
 state:=&State{Symbol:"600519.SH"}
 res,err:=(&BullResearcher{Model:m}).GenerateTurn(researchlang.RequireChinese(context.Background()),validArtifact(),state,1)
 if err!=nil{t.Fatal(err)}
 if m.calls!=2||res.Arguments[0].ID!="bull-r1-1"||res.Side!=SideBull{t.Fatalf("calls=%d turn=%+v",m.calls,res)}
 if len(state.Turns)!=0{t.Fatal("input state must remain unchanged")}
}
func TestBullTurnEnglishRepairExhaustion(t *testing.T){
 m:=&scriptedTurnModel{outputs:[]string{englishBullTurn,englishBullTurn}}
 _,err:=(&BullResearcher{Model:m}).GenerateTurn(researchlang.RequireChinese(context.Background()),validArtifact(),&State{Symbol:"600519.SH"},1)
 if !errors.Is(err,researchlang.ErrLanguage)||m.calls!=2{t.Fatalf("err=%v calls=%d",err,m.calls)}
}
