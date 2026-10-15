package application

import (
 "context"
 "encoding/json"
 "errors"
)

// ConversationResearchTools is the narrow typed capability boundary for R2.
// It delegates to the same ResearchRun manager used by the HTTP workspace;
// it never manufactures research output or bypasses Run authorization.
type ConversationResearchTools struct {
 runs interface {
  Submit(context.Context, ResearchRequest) (RunRecord, error)
  Get(context.Context, string) (RunRecord, error)
  List(context.Context) ([]RunRecord, error)
 }
}

func NewConversationResearchTools(runs *RunManager) (*ConversationResearchTools, error) {
 if runs == nil { return nil, errors.New("conversation tools require a run manager") }
 return &ConversationResearchTools{runs:runs}, nil
}

type ConversationRunSummary struct {
 RunID string `json:"run_id"`
 Symbol string `json:"symbol"`
 State RunState `json:"state"`
 FailureCode string `json:"failure_code,omitempty"`
}

func conversationSummary(rec RunRecord) ConversationRunSummary {
 return ConversationRunSummary{RunID:rec.ID,Symbol:rec.Symbol,State:rec.State,FailureCode:rec.FailureCode}
}

// StartResearch requires an already validated typed request constructed by the
// existing application boundary. Chat text alone is never interpreted as a
// financial fact, account snapshot, or instruction to trade.
func (t *ConversationResearchTools) StartResearch(ctx context.Context, req ResearchRequest) (ConversationRunSummary, error) {
 if t == nil || t.runs == nil { return ConversationRunSummary{}, errors.New("conversation tools unavailable") }
 rec, err := t.runs.Submit(ctx, req)
 if err != nil { return ConversationRunSummary{}, err }
 return conversationSummary(rec), nil
}

func (t *ConversationResearchTools) GetResearchRun(ctx context.Context, id string) (ConversationRunSummary, error) {
 if t == nil || t.runs == nil { return ConversationRunSummary{}, errors.New("conversation tools unavailable") }
 rec, err := t.runs.Get(ctx, id)
 if err != nil { return ConversationRunSummary{}, err }
 return conversationSummary(rec), nil
}

func (t *ConversationResearchTools) ListResearchRuns(ctx context.Context) ([]ConversationRunSummary, error) {
 if t == nil || t.runs == nil { return nil, errors.New("conversation tools unavailable") }
 runs, err := t.runs.List(ctx)
 if err != nil { return nil, err }
 result := make([]ConversationRunSummary, 0, len(runs))
 for _, rec := range runs { result = append(result,conversationSummary(rec)) }
 return result, nil
}

// ReadArtifact fails closed for unfinished or failed runs. Return an owned
// copy, never a mutable alias into the persisted Run snapshot.
func (t *ConversationResearchTools) ReadArtifact(ctx context.Context, id string) (json.RawMessage, error) {
 if t == nil || t.runs == nil { return nil, errors.New("conversation tools unavailable") }
 rec, err := t.runs.Get(ctx,id)
 if err != nil { return nil,err }
 if rec.State != RunCompleted || len(rec.Artifact)==0 { return nil,errors.New("completed research artifact unavailable") }
 return append(json.RawMessage(nil),rec.Artifact...),nil
}

// ReadDebate selects only the canonical validated debate_state from the
// completed Portfolio Review artifact. Decision-only runs have no debate.
// Never derive arguments from free-form TradeIntent evidence or model prose.
func (t *ConversationResearchTools) ReadDebate(ctx context.Context, id string) (json.RawMessage, error) {
 artifact, err := t.ReadArtifact(ctx, id)
 if err != nil { return nil, err }
 var root struct {
  PortfolioReview *struct {
   Debate json.RawMessage `json:"debate_state"`
  } `json:"portfolio_review"`
 }
 if err := json.Unmarshal(artifact, &root); err != nil { return nil, errors.New("invalid persisted research artifact") }
 if root.PortfolioReview == nil || len(root.PortfolioReview.Debate)==0 || string(root.PortfolioReview.Debate)=="null" {
  return nil, errors.New("validated debate unavailable for this research run")
 }
 if root.PortfolioReview.Debate[0]!='{' { return nil, errors.New("invalid persisted debate state") }
 return append(json.RawMessage(nil),root.PortfolioReview.Debate...),nil
}
