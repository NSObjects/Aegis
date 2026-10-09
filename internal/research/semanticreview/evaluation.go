package semanticreview

import "fmt"

// GoldLabel is an independent, manually annotated expectation. It must not be
// inferred from reference alignment or from the report being scored.
type GoldLabel struct {
	InsightIndex int `json:"insight_index"`
	ArgumentID string `json:"argument_id"`
	ExpectedVerdict Verdict `json:"expected_verdict"`
}

// EvaluationMetrics is agreement with supplied labels, NOT semantic accuracy
// on real markets and NOT a guarantee that human annotations are correct.
type EvaluationMetrics struct {
	Total int `json:"total"`
	ExactMatches int `json:"exact_matches"`
	PredictedUncertain int `json:"predicted_uncertain"`
	FalseConsistentOnContradictory int `json:"false_consistent_on_contradictory"`
	FalseContradictoryOnConsistent int `json:"false_contradictory_on_consistent"`
}

// ScoreLabeledReport compares a validated report against *independent*
// identity-checked, ordered human labels. This does not invoke a model, alter
// a ResearchPlan, or feed any score back into portfolio decisions.
func ScoreLabeledReport(tasks *Tasks, report *Report, gold []GoldLabel) (*EvaluationMetrics,error) {
	if err:=ValidateReport(tasks,report);err!=nil{
		return nil,fmt.Errorf("invalid report for evaluation: %w",err)
	}
	if len(gold)!=len(tasks.Items){
		return nil,fmt.Errorf("gold label count %d does not match task count %d",len(gold),len(tasks.Items))
	}
	metrics:=&EvaluationMetrics{Total:len(gold)}
	for i,label:=range gold {
		task:=tasks.Items[i]
		if label.InsightIndex!=task.InsightIndex||label.ArgumentID!=task.ArgumentID{
			return nil,fmt.Errorf("gold label %d identity differs from task",i)
		}
		switch label.ExpectedVerdict{
		case VerdictConsistent,VerdictContradictory,VerdictUncertain:
		default:
			return nil,fmt.Errorf("gold label %d has unsupported verdict %q",i,label.ExpectedVerdict)
		}
		prediction:=report.Findings[i].Verdict
		if prediction==label.ExpectedVerdict{metrics.ExactMatches++}
		if prediction==VerdictUncertain{metrics.PredictedUncertain++}
		if prediction==VerdictConsistent&&label.ExpectedVerdict==VerdictContradictory{
			metrics.FalseConsistentOnContradictory++
		}
		if prediction==VerdictContradictory&&label.ExpectedVerdict==VerdictConsistent{
			metrics.FalseContradictoryOnConsistent++
		}
	}
	return metrics,nil
}
