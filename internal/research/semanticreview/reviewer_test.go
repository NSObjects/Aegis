package semanticreview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type reviewerModelStub struct {
	output  string
	calls   int
	input   []*schema.AgenticMessage
	run     func(context.Context) (*schema.AgenticMessage, error)
}

func (m *reviewerModelStub) Generate(ctx context.Context, messages []*schema.AgenticMessage, _ ...model.Option) (*schema.AgenticMessage, error) {
	m.calls++
	m.input = messages
	if m.run != nil { return m.run(ctx) }
	return &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.AssistantGenText{Text:m.output}),
		},
	},nil
}
func (m *reviewerModelStub) Stream(context.Context, []*schema.AgenticMessage, ...model.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	panic("streaming is not used for strict semantic review")
}

func validReviewerTasks(t *testing.T) *Tasks {
	t.Helper()
	a,d,p := semanticFixture()
	tasks,err := BuildTasks(a,d,p)
	if err!=nil{t.Fatal(err)}
	return tasks
}

func validReviewJSON(t *testing.T, tasks *Tasks) string {
	t.Helper()
	findings := make([]Finding,0,len(tasks.Items))
	for _,item := range tasks.Items {
		findings=append(findings,Finding{
			InsightIndex:item.InsightIndex,ArgumentID:item.ArgumentID,
			Verdict:VerdictUncertain,Rationale:"not enough evidence for entailment",
			Limitations:"only pre-existing research claims and references are available",
		})
	}
	data,err:=json.Marshal(Report{Symbol:tasks.Symbol,Findings:findings})
	if err!=nil{t.Fatal(err)}
	return string(data)
}

func TestOptionalEinoReviewerProducesOnlyValidatedAdvisoryReport(t *testing.T) {
	tasks:=validReviewerTasks(t)
	modelStub:=&reviewerModelStub{output:validReviewJSON(t,tasks)}
	reviewer:=&Reviewer{Model:modelStub,Timeout:time.Second,MaxTasks:8}
	result,err:=reviewer.Review(context.Background(),tasks)
	if err!=nil{t.Fatal(err)}
	if err:=ValidateReport(tasks,result);err!=nil {t.Fatalf("unvalidated report: %v",err)}
	if modelStub.calls!=1||len(modelStub.input)!=2 {t.Fatalf("calls=%d inputs=%d",modelStub.calls,len(modelStub.input))}
	if !strings.Contains(fmt.Sprint(modelStub.input),"reference") {
		t.Fatal("model did not receive task identities and reference-alignment instructions")
	}
	if len(result.Findings)!=3||result.Findings[0].Verdict!=VerdictUncertain {
		t.Fatalf("verdict was invented or lost: %+v",result)
	}
}

func TestOptionalEinoReviewerSkipsModelWhenNoLinks(t *testing.T) {
	tasks:=&Tasks{Symbol:"600519.SH"}
	modelStub:=&reviewerModelStub{run:func(context.Context)(*schema.AgenticMessage,error){
		return nil,errors.New("should never be called")
	}}
	result,err:=(&Reviewer{Model:modelStub}).Review(context.Background(),tasks)
	if err!=nil||result==nil||result.Symbol!="600519.SH"||len(result.Findings)!=0||modelStub.calls!=0 {
		t.Fatalf("empty tasks unexpectedly called model: report=%+v err=%v calls=%d",result,err,modelStub.calls)
	}
}

func TestOptionalEinoReviewerRejectsBadInputsBeforeModel(t *testing.T) {
	tasks:=validReviewerTasks(t)
	tests:=[]struct{
		name string
		tasks *Tasks
		model model.BaseModel[*schema.AgenticMessage]
		max int
		want string
	}{
		{"nil tasks",nil,&reviewerModelStub{} ,8,"tasks"},
		{"empty symbol",&Tasks{Items:tasks.Items},&reviewerModelStub{},8,"symbol"},
		{"missing model",tasks,nil,8,"model"},
		{"capacity",tasks,&reviewerModelStub{},1,"limit"},
		{"missing argument ID",&Tasks{Symbol:"600519.SH",Items:[]Task{{InsightIndex:0,ArgumentID:""}}},&reviewerModelStub{},8,"argument_id"},
	}
	for _,tc:=range tests{
		t.Run(tc.name,func(t *testing.T){
			out,err:=(&Reviewer{Model:tc.model,MaxTasks:tc.max}).Review(context.Background(),tc.tasks)
			if out!=nil||err==nil||!strings.Contains(strings.ToLower(err.Error()),tc.want){
				t.Fatalf("report=%+v err=%v want %q",out,err,tc.want)
			}
		})
	}
	ctx,cancel:=context.WithCancel(context.Background())
	cancel()
	m:=&reviewerModelStub{}
	got,err:=(&Reviewer{Model:m}).Review(ctx,tasks)
	if got!=nil||!errors.Is(err,context.Canceled)||m.calls!=0 {
		t.Fatalf("cancelled input called model or lost error: report=%+v err=%v calls=%d",got,err,m.calls)
	}
}

func TestOptionalEinoReviewerRejectsUnsafeModelResponses(t *testing.T) {
	tasks:=validReviewerTasks(t)
	good:=validReviewJSON(t,tasks)
	cases:=[]struct{name string;data string;want string}{
		{"json markdown fence","\x60\x60\x60json\n"+good+"\n\x60\x60\x60","JSON"},
		{"multiple top-level objects",good+" "+good,"JSON"},
		{"unknown field",strings.TrimSuffix(good,"}")+`,"extra":true}`,"JSON"},
		{"truncated",good[:len(good)-1],"JSON"},
		{"wrong symbol",strings.Replace(good,"600519.SH","000001.SZ",1),"symbol"},
		{"invented id",strings.Replace(good,"bear-1","invented-id",1),"argument_id"},
		{"fake verdict",strings.Replace(good,"UNCERTAIN","CERTAIN",1),"verdict"},
		{"missing uncertainty",strings.Replace(good,"only pre-existing research claims and references are available","",1),"limitations"},
	}
	for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
		m:=&reviewerModelStub{output:tc.data}
		got,err:=(&Reviewer{Model:m}).Review(context.Background(),tasks)
		if got!=nil||err==nil||!strings.Contains(strings.ToLower(err.Error()),strings.ToLower(tc.want)){
			t.Fatalf("accepted unsafe response: result=%+v err=%v want=%q",got,err,tc.want)
		}
	})}
	for _,tc:=range []struct{name string;msg *schema.AgenticMessage}{
		{"nil message",nil},
		{"non assistant",&schema.AgenticMessage{Role:schema.AgenticRoleTypeUser}},
		{"empty blocks",&schema.AgenticMessage{Role:schema.AgenticRoleTypeAssistant}},
		{"mixed blocks",&schema.AgenticMessage{Role:schema.AgenticRoleTypeAssistant,ContentBlocks:[]*schema.ContentBlock{
			schema.NewContentBlock(&schema.AssistantGenText{Text:good}),{},
		}}},
	}{
		t.Run(tc.name,func(t *testing.T){
			m:=&reviewerModelStub{run:func(context.Context)(*schema.AgenticMessage,error){return tc.msg,nil}}
			got,err:=(&Reviewer{Model:m}).Review(context.Background(),tasks)
			if got!=nil||err==nil {t.Fatalf("accepted unexpected model content: %+v %v",got,err)}
		})
	}
}

func TestOptionalEinoReviewerPropagatesTimeoutAndProviderFailureWithoutFakeVerdict(t *testing.T) {
	tasks:=validReviewerTasks(t)
	blocked:=&reviewerModelStub{run:func(ctx context.Context)(*schema.AgenticMessage,error){
		<-ctx.Done()
		return nil,ctx.Err()
	}}
	result,err:=(&Reviewer{Model:blocked,Timeout:5*time.Millisecond}).Review(context.Background(),tasks)
	if result!=nil||!errors.Is(err,context.DeadlineExceeded)||blocked.calls!=1 {
		t.Fatalf("timeout fabricated result or lost cause: result=%+v err=%v calls=%d",result,err,blocked.calls)
	}
	upstream:=errors.New("provider unavailable")
	failing:=&reviewerModelStub{run:func(context.Context)(*schema.AgenticMessage,error){return nil,upstream}}
	result,err=(&Reviewer{Model:failing}).Review(context.Background(),tasks)
	if result!=nil||!errors.Is(err,upstream) {t.Fatalf("provider failure hidden by fallback: %+v %v",result,err)}
}
