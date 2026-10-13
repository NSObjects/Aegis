package debate

import (
	"Aegis/internal/research/synthesis"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type scriptedTurnModel struct {
	outputs []string
	err error
	calls int
	inputs [][]*schema.AgenticMessage
}

func (m *scriptedTurnModel) Generate(
	_ context.Context,
	input []*schema.AgenticMessage,
	_ ...model.Option,
) (*schema.AgenticMessage, error) {
	m.calls++
	m.inputs = append(m.inputs, append([]*schema.AgenticMessage(nil), input...))
	if m.err != nil { return nil, m.err }
	if m.calls > len(m.outputs) {
		return nil, errors.New("unexpected third model call")
	}
	return &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.AssistantGenText{Text:m.outputs[m.calls-1]}),
		},
	}, nil
}

func (m *scriptedTurnModel) Stream(context.Context, []*schema.AgenticMessage, ...model.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	panic("unexpected stream")
}

const validBullTurn = `{"round":1,"side":"BULL","thesis":"grounded positive thesis","arguments":[{"id":"bull-r1-1","claim":"grounded technical argument","references":[{"domain":"TECHNICAL"}]}],"confidence":0.7}`
const missingBullArgumentID = `{"round":1,"side":"BULL","thesis":"grounded positive thesis","arguments":[{"claim":"technical evidence exists","references":[{"domain":"TECHNICAL"}]}],"confidence":0.7}`

const validBearTurn = `{"round":1,"side":"BEAR","thesis":"downside exists","arguments":[{"id":"bear-r1-1","claim":"known market risk","references":[{"domain":"MARKET"}]}],"rebuttals":[{"claim":"known downside","target_argument_id":"bull-r1-1","references":[{"domain":"MARKET"}]}],"confidence":0.6}`

func TestBullResearcherCorrectsMissingIDWithRealModelOutput(t *testing.T) {
	m:=&scriptedTurnModel{outputs:[]string{missingBullArgumentID,validBullTurn}}
	state:=&State{Symbol:"600519.SH"}
	bull:=&BullResearcher{Model:m}
	turn,err:=bull.GenerateTurn(context.Background(),validArtifact(),state,1)
	if err!=nil {t.Fatalf("GenerateTurn error=%v",err)}
	if m.calls!=2 || len(m.inputs)!=2 || len(m.inputs[0])!=2 ||
		len(m.inputs[1])!=3 {
		t.Fatalf("repair must be exactly one model correction request; calls=%d inputs=%d",m.calls,len(m.inputs))
	}
	if !strings.Contains(m.inputs[1][2].ContentBlocks[0].UserInputText.Text,"argument 0 id is required") {
		t.Fatal("retry must explain actual domain validation failure")
	}
	if turn.Arguments[0].ID!="bull-r1-1" ||
		len(state.Turns)!=0 {
		t.Fatalf("only model-generated IDs accepted; state must not mutate: %+v",turn)
	}
}

func TestBullResearcherStillRejectsMalformedSecondTurn(t *testing.T) {
	m:=&scriptedTurnModel{outputs:[]string{missingBullArgumentID,missingBullArgumentID}}
	got,err:=(&BullResearcher{Model:m}).GenerateTurn(context.Background(),validArtifact(),&State{Symbol:"600519.SH"},1)
	if err==nil || !strings.Contains(err.Error(),"argument 0 id is required") ||
		!strings.Contains(err.Error(),"after 2 attempts") || len(got.Arguments)>0 || m.calls!=2 {
		t.Fatalf("invalid arguments must fail closed: calls=%d got=%+v err=%v",m.calls,got,err)
	}
}

func TestBearResearcherCorrectsUnknownPriorArgumentID(t *testing.T) {
	state:=&State{Symbol:"600519.SH",Turns:[]Turn{{
		Round:1,Side:SideBull,Thesis:"positive",
		Arguments:[]Argument{{ID:"bull-r1-1",Claim:"grounded",References:[]synthesis.Reference{{Domain:synthesis.DomainTechnical}}}},
		Confidence:.7,
	}}}
	wrong:=`{"round":1,"side":"BEAR","thesis":"downside","arguments":[{"id":"bear-r1-1","claim":"risk","references":[{"domain":"MARKET"}]}],"rebuttals":[{"claim":"invalid ref","target_argument_id":"invented-bull","references":[{"domain":"MARKET"}]}],"confidence":0.6}`
	m:=&scriptedTurnModel{outputs:[]string{wrong,validBearTurn}}
	turn,err:=(&BearResearcher{Model:m}).GenerateTurn(context.Background(),validArtifact(),state,1)
	if err!=nil || m.calls!=2 || len(turn.Rebuttals)!=1 ||
		turn.Rebuttals[0].TargetArgumentID!="bull-r1-1" {
		t.Fatalf("rebuttal correction must cite true historical argument: calls=%d turn=%+v err=%v",m.calls,turn,err)
	}
	if len(state.Turns)!=1 {
		t.Fatal("researchers must never mutate shared state")
	}
}

func TestDebateRepairCannotBypassReferenceValidation(t *testing.T) {
	bad:=`{"round":1,"side":"BULL","thesis":"false","arguments":[{"id":"b-1","claim":"invented event","references":[{"domain":"EVENT","event_id":"fake"}]}],"confidence":0.6}`
	m:=&scriptedTurnModel{outputs:[]string{bad,bad}}
	_,err:=(&BullResearcher{Model:m}).GenerateTurn(context.Background(),validArtifact(),&State{Symbol:"600519.SH"},1)
	if err==nil || !strings.Contains(err.Error(),"does not exist in ResearchArtifact") || m.calls!=2 {
		t.Fatalf("ungrounded references must still fail: calls=%d err=%v",m.calls,err)
	}
}

func TestDebateRepairRetriesProviderFailureOnce(t *testing.T) {
	originalDelay := debateProviderRetryDelay
	debateProviderRetryDelay = 0
	defer func() { debateProviderRetryDelay = originalDelay }()

	m:=&scriptedTurnModel{err:errors.New("provider unavailable")}
	_,err:=(&BullResearcher{Model:m}).GenerateTurn(context.Background(),validArtifact(),&State{Symbol:"600519.SH"},1)
	if err==nil || !strings.Contains(err.Error(),"provider unavailable") || m.calls!=2 {
		t.Fatalf("transient provider failure must be retried exactly once: calls=%d err=%v",m.calls,err)
	}
}

func TestDebateRepairDoesNotInventForAbsentOutput(t *testing.T) {
	m:=&scriptedTurnModel{outputs:[]string{"",""}}
	_,err:=(&BullResearcher{Model:m}).GenerateTurn(context.Background(),validArtifact(),&State{Symbol:"600519.SH"},1)
	if err==nil || !strings.Contains(err.Error(),"empty output") || m.calls!=2 {
		t.Fatalf("an empty turn is not a legitimate research conclusion: calls=%d err=%v",m.calls,err)
	}
}

func TestBullRoundTwoRepairsMissingBearEngagement(t *testing.T) {
 state:=&State{Symbol:"600519.SH",Turns:[]Turn{
  {Round:1,Side:SideBull,Thesis:"positive",Arguments:[]Argument{{ID:"bull-r1-1",Claim:"technical support",References:[]synthesis.Reference{{Domain:synthesis.DomainTechnical}}}},Confidence:.7},
  {Round:1,Side:SideBear,Thesis:"risk",Arguments:[]Argument{{ID:"bear-r1-1",Claim:"market uncertainty",References:[]synthesis.Reference{{Domain:synthesis.DomainMarket}}}},Rebuttals:[]Rebuttal{{Claim:"countercase",TargetArgumentID:"bull-r1-1",References:[]synthesis.Reference{{Domain:synthesis.DomainMarket}}}},Confidence:.6},
 }}
 noEngagement:=`{"round":2,"side":"BULL","thesis":"bull argument","arguments":[{"id":"bull-r2-1","claim":"technical support","references":[{"domain":"TECHNICAL"}]}],"confidence":0.7}`
 withEngagement:=`{"round":2,"side":"BULL","thesis":"bull argument","arguments":[{"id":"bull-r2-1","claim":"technical support","references":[{"domain":"TECHNICAL"}]}],"rebuttals":[{"claim":"market uncertainty can be weighed against observed technical facts","target_argument_id":"bear-r1-1","references":[{"domain":"MARKET"}]}],"confidence":0.7}`
 m:=&scriptedTurnModel{outputs:[]string{noEngagement,withEngagement}}
 turn,err:=(&BullResearcher{Model:m}).GenerateTurn(context.Background(),validArtifact(),state,2)
 if err!=nil || m.calls!=2 || len(turn.Rebuttals)!=1 || turn.Rebuttals[0].TargetArgumentID!="bear-r1-1" {
  t.Fatalf("expected model-grounded retry to engage prior BEAR: turn=%+v calls=%d err=%v",turn,m.calls,err)
 }
 if !strings.Contains(m.inputs[1][2].ContentBlocks[0].UserInputText.Text,"must rebut at least one prior BEAR argument"){
  t.Fatal("retry must explain opposing engagement requirement")
 }
 if len(state.Turns)!=2{t.Fatal("repair must not mutate debate history")}
}

func TestBullRoundTwoFailsClosedWithoutEngagement(t *testing.T){
 state:=&State{Symbol:"600519.SH",Turns:[]Turn{
  {Round:1,Side:SideBull,Thesis:"positive",Arguments:[]Argument{{ID:"bull-r1-1",Claim:"technical support",References:[]synthesis.Reference{{Domain:synthesis.DomainTechnical}}}},Confidence:.7},
  {Round:1,Side:SideBear,Thesis:"risk",Arguments:[]Argument{{ID:"bear-r1-1",Claim:"market uncertainty",References:[]synthesis.Reference{{Domain:synthesis.DomainMarket}}}},Rebuttals:[]Rebuttal{{Claim:"countercase",TargetArgumentID:"bull-r1-1",References:[]synthesis.Reference{{Domain:synthesis.DomainMarket}}}},Confidence:.6},
 }}
 noEngagement:=`{"round":2,"side":"BULL","thesis":"bull argument","arguments":[{"id":"bull-r2-1","claim":"technical support","references":[{"domain":"TECHNICAL"}]}],"confidence":0.7}`
 m:=&scriptedTurnModel{outputs:[]string{noEngagement,noEngagement}}
 turn,err:=(&BullResearcher{Model:m}).GenerateTurn(context.Background(),validArtifact(),state,2)
 if err==nil || m.calls!=2 || !strings.Contains(err.Error(),"must rebut at least one prior BEAR argument")||len(turn.Arguments)>0{
  t.Fatalf("must fail closed without model-grounded engagement: turn=%+v calls=%d err=%v",turn,m.calls,err)
 }
}
