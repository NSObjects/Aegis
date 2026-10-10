package risk

import (
	"Aegis/internal/portfolio"
	riskctx "Aegis/internal/risk"
	"Aegis/internal/tradeintent"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type modelStub struct {
	output string
	calls  int
	inputs [][]*schema.AgenticMessage
}

func (m *modelStub) Generate(
	_ context.Context,
	messages []*schema.AgenticMessage,
	_ ...model.Option,
) (*schema.AgenticMessage, error) {
	m.calls++
	m.inputs = append(m.inputs, messages)
	return &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.AssistantGenText{Text: m.output}),
		},
	}, nil
}

func (m *modelStub) Stream(
	context.Context,
	[]*schema.AgenticMessage,
	...model.Option,
) (*schema.StreamReader[*schema.AgenticMessage], error) {
	panic("unexpected Stream call")
}

func testIntent() *tradeintent.Intent {
	return &tradeintent.Intent{
		Symbol:     "600519.SH",
		Thesis:     "research thesis",
		Evidence:   []string{"grounded evidence"},
		Risk:       []string{"known downside"},
		Status:     tradeintent.StatusCandidate,
		Confidence: 0.68,
	}
}

func testPositionsPortfolio() *portfolio.Context {
	return &portfolio.Context{
		State: portfolio.StatePositions,
		AsOf:  "2026-10-08T09:00:00+08:00",
		Positions: []portfolio.Position{
			{Symbol: "600519.SH", Quantity: 100},
			{Symbol: "000001.SZ", Quantity: 200},
		},
	}
}

func TestAnalystBuildsGroundedRiskContext(t *testing.T) {
	m := &modelStub{output: `{
		"symbol":"600519.SH",
		"assessment":"CAUTION",
		"summary":"existing position increases the importance of the known downside risk",
		"findings":[
			{
				"claim":"the candidate intent carries a known downside risk",
				"references":[{"domain":"TRADE_INTENT","symbol":"600519.SH"}]
			}
		],
		"constraints":[
			{
				"claim":"the existing position should be considered by the downstream portfolio layer",
				"references":[{"domain":"PORTFOLIO","symbol":"600519.SH"}]
			}
		],
		"confidence":0.72
	}`}
	a := &Analyst{Model: m}

	result, err := a.Review(context.Background(), testIntent(), testPositionsPortfolio())
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if result.Assessment != riskctx.AssessmentCaution {
		t.Fatalf("assessment = %q", result.Assessment)
	}
	if m.calls != 1 {
		t.Fatalf("model calls = %d, want 1", m.calls)
	}
}

func TestAnalystNotProvidedIsDeterministic(t *testing.T) {
	m := &modelStub{}
	a := &Analyst{Model: m}

	result, err := a.Review(
		context.Background(),
		testIntent(),
		&portfolio.Context{State: portfolio.StateNotProvided},
	)
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if result.Assessment != riskctx.AssessmentInsufficientContext {
		t.Fatalf("assessment = %q", result.Assessment)
	}
	if m.calls != 0 {
		t.Fatalf("model calls = %d, want 0", m.calls)
	}
}

func TestAnalystRejectsInventedPositionReference(t *testing.T) {
	m := &modelStub{output: `{
		"symbol":"600519.SH",
		"assessment":"HIGH_RISK",
		"summary":"invented concentration",
		"findings":[
			{
				"claim":"invented position",
				"references":[{"domain":"PORTFOLIO","symbol":"300750.SZ"}]
			}
		],
		"confidence":0.8
	}`}
	a := &Analyst{Model: m}

	_, err := a.Review(context.Background(), testIntent(), testPositionsPortfolio())
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("error = %v", err)
	}
}

func TestAnalystRejectsUnknownOutputField(t *testing.T) {
	m := &modelStub{output: `{
		"symbol":"600519.SH",
		"assessment":"CAUTION",
		"summary":"test",
		"confidence":0.5,
		"action":"AVOID"
	}`}
	a := &Analyst{Model: m}

	_, err := a.Review(context.Background(), testIntent(), testPositionsPortfolio())
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v", err)
	}
}

func TestAnalystRequiresModelForObservedPortfolio(t *testing.T) {
	a := &Analyst{}
	_, err := a.Review(
		context.Background(),
		testIntent(),
		&portfolio.Context{
			State: portfolio.StateEmpty,
			AsOf:  "2026-10-08T09:00:00+08:00",
		},
	)
	if err == nil || !strings.Contains(err.Error(), "model is required") {
		t.Fatalf("error = %v", err)
	}
}

func TestAnalystReceivesOnlyReconciledObservedExposure(t *testing.T) {
	m := &modelStub{output: `{"symbol":"600519.SH","assessment":"CAUTION","summary":"consider existing concentration","confidence":0.7}`}
	book := testPositionsPortfolio()
	book.Valuation = &portfolio.Valuation{
		Scope: portfolio.ValuationScopeCashAndPositions,
		Currency: portfolio.CurrencyCNY,
		TotalValueFen: 100000,
		CashFen: 30000,
		PositionValues: []portfolio.PositionValue{
			{Symbol:"600519.SH",MarketValueFen:60000},
			{Symbol:"000001.SZ",MarketValueFen:10000},
		},
	}
	result,err:=(&Analyst{Model:m}).Review(context.Background(),testIntent(),book)
	if err!=nil || result==nil || m.calls!=1 {t.Fatalf("Review err=%v result=%+v calls=%d",err,result,m.calls)}
	if len(m.inputs)!=1 || len(m.inputs[0])!=2 ||
		len(m.inputs[0][1].ContentBlocks)!=1 || m.inputs[0][1].ContentBlocks[0].UserInputText==nil {
		t.Fatalf("model input has unexpected layout")
	}
	var input struct {
		ObservedExposure *portfolio.Exposure `json:"observed_exposure"`
	}
	raw:=m.inputs[0][1].ContentBlocks[0].UserInputText.Text
	if err:=json.Unmarshal([]byte(raw),&input);err!=nil {t.Fatal(err)}
	if input.ObservedExposure==nil ||
		input.ObservedExposure.TotalValueFen!=100000 ||
		input.ObservedExposure.CashWeightBps==nil ||
		*input.ObservedExposure.CashWeightBps!=3000 ||
		len(input.ObservedExposure.Positions)!=2 ||
		input.ObservedExposure.Positions[0].WeightBps!=6000 {
		t.Fatalf("risk model did not receive deterministic observed exposure: %+v",input.ObservedExposure)
	}
}

func TestAnalystCannotInferExposureFromQuantity(t *testing.T) {
	m:=&modelStub{output:`{"symbol":"600519.SH","assessment":"INSUFFICIENT_CONTEXT","summary":"no monetary values","confidence":1}`}
	_,err:=(&Analyst{Model:m}).Review(context.Background(),testIntent(),testPositionsPortfolio())
	if err!=nil {t.Fatal(err)}
	raw:=m.inputs[0][1].ContentBlocks[0].UserInputText.Text
	if strings.Contains(raw,"observed_exposure") {
		t.Fatalf("unknown valuation must not become derived exposure: %s",raw)
	}
}

func TestAnalystRejectsUnbalancedValuationWithoutModelCall(t *testing.T) {
	m:=&modelStub{}
	book:=testPositionsPortfolio()
	book.Valuation=&portfolio.Valuation{
		Scope:portfolio.ValuationScopeCashAndPositions, Currency:portfolio.CurrencyCNY,
		TotalValueFen:1, CashFen:1,
	}
	_,err:=(&Analyst{Model:m}).Review(context.Background(),testIntent(),book)
	if err==nil || m.calls!=0 || !strings.Contains(err.Error(),"valuation") {
		t.Fatalf("unreconciled account must fail before LLM: calls=%d err=%v",m.calls,err)
	}
}

func TestAnalystAllowsExactJSONModeMetadata(t *testing.T) {
 m:=&modelStub{output:`{"type":"json_object","symbol":"600519.SH","assessment":"INSUFFICIENT_CONTEXT","summary":"Exposure is unknown without portfolio valuation","confidence":0.5}`}
 got,err:=(&Analyst{Model:m}).Review(context.Background(),testIntent(),testPositionsPortfolio())
 if err!=nil||got==nil||got.Assessment!=riskctx.AssessmentInsufficientContext {
  t.Fatalf("valid JSON-mode metadata must not invalidate otherwise grounded risk output: %+v %v",got,err)
 }
}
func TestAnalystRejectsUnexpectedJSONModeType(t *testing.T) {
 for _,value:=range []string{`"unexpected"`,`17`,`null`} {
  m:=&modelStub{output:`{"type":`+value+`,"symbol":"600519.SH","assessment":"CAUTION","summary":"x","confidence":0.5}`}
  got,err:=(&Analyst{Model:m}).Review(context.Background(),testIntent(),testPositionsPortfolio())
  if err==nil||got!=nil||!strings.Contains(err.Error(),"invalid type metadata"){
   t.Fatalf("type=%s unexpectedly accepted: %+v, %v",value,got,err)
  }
 }
}
func TestAnalystRejectsExtraFieldEvenWithJSONModeMetadata(t *testing.T){
 m:=&modelStub{output:`{"type":"json_object","symbol":"600519.SH","assessment":"CAUTION","summary":"x","confidence":0.5,"action":"BUY"}`}
 got,err:=(&Analyst{Model:m}).Review(context.Background(),testIntent(),testPositionsPortfolio())
 if got!=nil||err==nil||!strings.Contains(err.Error(),"unknown field"){
  t.Fatalf("other unknown fields must fail closed: %+v %v",got,err)
 }
}
