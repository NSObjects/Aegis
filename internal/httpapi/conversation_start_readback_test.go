package httpapi

import (
 "context"
 "encoding/json"
 "errors"
 "net/http"
 "strings"
 "testing"
 "time"

 "Aegis/internal/application"
)

type conversationSubmitFixture struct {
 testConversationResponder
 runs *fakeRuns
 fail bool
}
func (s *conversationSubmitFixture) StartConfirmedResearch(ctx context.Context, _ string, symbol string) (application.ConversationRunSummary,error) {
 if s.fail {return application.ConversationRunSummary{},errors.New("synthetic run submission failure")}
 input,err:=application.NewLiveResearchInput(symbol,"",time.Date(2026,10,16,0,0,0,0,time.UTC),48*time.Hour)
 if err!=nil{return application.ConversationRunSummary{},err}
 rec,err:=s.runs.Submit(ctx,application.ResearchRequest{Mode:application.ModeDecision,Input:input})
 if err!=nil{return application.ConversationRunSummary{},err}
 return application.ConversationRunSummary{RunID:rec.ID,Symbol:rec.Symbol,State:rec.State},nil
}

func TestConfirmedConversationHTTPStartThenReadRun(t *testing.T) {
 id:=strings.Repeat("a",32)
 runs:=&fakeRuns{rec:application.RunRecord{ID:id,Symbol:"600519.SH",Mode:application.ModeDecision,State:application.RunPending,CreatedAt:time.Date(2026,10,16,0,0,0,0,time.UTC)}}
 handler:=mkHandler(t,runs,application.ModeDecision)
 responder:=&conversationSubmitFixture{runs:runs}
 handler.WithConversationResponder(responder)
 start:=apiRequest(t,handler,http.MethodPost,"/api/v1/conversation/query",`{"message":"现在研究贵州茅台","start_symbol":"600519.SH","confirm_start":true}`,true,nil)
 if start.Code!=http.StatusOK{t.Fatalf("start HTTP %d: %s",start.Code,start.Body.String())}
 var created struct{RunID string `json:"run_id"`;Symbol string `json:"symbol"`;State string `json:"state"`}
 if err:=json.Unmarshal(start.Body.Bytes(),&created);err!=nil{t.Fatal(err)}
 if created.RunID!=id||created.Symbol!="600519.SH"||created.State!="PENDING" {t.Fatalf("unexpected created Run: %+v",created)}
 detail:=apiRequest(t,handler,http.MethodGet,"/api/v1/research/runs/"+created.RunID,"",true,nil)
 if detail.Code!=http.StatusOK {t.Fatalf("created Run not queryable, status=%d, body=%s",detail.Code,detail.Body.String())}
 if !strings.Contains(detail.Body.String(),id)||!strings.Contains(detail.Body.String(),"600519.SH"){t.Fatalf("readback mismatch: %s",detail.Body.String())}
 if runs.submitCalls!=1||runs.lastRequest.Mode!=application.ModeDecision||runs.lastRequest.Input.Symbol!="600519.SH"||runs.lastRequest.Portfolio!=nil{t.Fatalf("unexpected Run submission: %d %+v",runs.submitCalls,runs.lastRequest)}
 denied:=apiRequest(t,handler,http.MethodPost,"/api/v1/conversation/query",`{"message":"研究其他股票","start_symbol":"000001.SH","confirm_start":true}`,true,nil)
 if denied.Code!=http.StatusBadRequest||runs.submitCalls!=1{t.Fatalf("unauthorized request caused submit: status=%d count=%d",denied.Code,runs.submitCalls)}
 responder.fail=true
 failed:=apiRequest(t,handler,http.MethodPost,"/api/v1/conversation/query",`{"message":"再研究一次","start_symbol":"600519.SH","confirm_start":true}`,true,nil)
 if failed.Code!=http.StatusServiceUnavailable||strings.Contains(failed.Body.String(),id){t.Fatalf("failed submit leaked success: %d %s",failed.Code,failed.Body.String())}
}
