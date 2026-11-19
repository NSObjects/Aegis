package portfolio

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const ResearchAccountSchemaVersion = 1

type ResearchSourceKind string

const (
	ResearchSourceOperator ResearchSourceKind = "OPERATOR"
	ResearchSourceProvider ResearchSourceKind = "PROVIDER"
)

// ResearchSource records when an input became available to the research
// process. The adapter that supplies it is responsible for source verification.
type ResearchSource struct {
	Kind        ResearchSourceKind `json:"kind"`
	Reference   string             `json:"reference"`
	AvailableAt time.Time          `json:"available_at"`
}

// CostFen is the observed total acquisition cost for the held quantity.
// A missing entry means cost is unknown. An explicit zero is preserved.
type PositionCost struct {
	Symbol  string `json:"symbol"`
	CostFen *int64 `json:"cost_fen"`
}

type ResearchAccountSnapshot struct {
	AccountID string          `json:"account_id,omitempty"`
	Version   string          `json:"version,omitempty"`
	Source    *ResearchSource `json:"source,omitempty"`
	Book      Context         `json:"book"`
	Costs     []PositionCost  `json:"costs,omitempty"`
}

// LossBudgetFen is the caller's declared loss capacity for this policy
// version. Cash, account value and other risk rules cannot substitute for it.
type ResearchRiskPolicy struct {
	Version       string         `json:"version"`
	Source        ResearchSource `json:"source"`
	Limits        Limits         `json:"limits"`
	LossBudgetFen *int64         `json:"loss_budget_fen,omitempty"`
}

type ResearchPosition struct {
	Position
	CostFen *int64 `json:"cost_fen,omitempty"`
}

type ResearchAccountView struct {
	SchemaVersion  int                `json:"schema_version"`
	RequestedAsOf  time.Time          `json:"requested_as_of"`
	State          State              `json:"state"`
	AccountID      string             `json:"account_id,omitempty"`
	AccountVersion string             `json:"account_version,omitempty"`
	AccountSource  *ResearchSource    `json:"account_source,omitempty"`
	AccountAsOf    string             `json:"account_as_of,omitempty"`
	Positions      []ResearchPosition `json:"positions,omitempty"`
	Exposure       *Exposure          `json:"exposure,omitempty"`
	PolicyVersion  string             `json:"policy_version,omitempty"`
	PolicySource   *ResearchSource    `json:"policy_source,omitempty"`
	LossBudgetFen  *int64             `json:"loss_budget_fen,omitempty"`
	LimitReview    *LimitReview       `json:"limit_review,omitempty"`
}

func validResearchReference(value string, max int) bool {
	return value != "" && len(value) <= max && strings.TrimSpace(value) == value &&
		!strings.ContainsAny(value, "\r\n\x00")
}

func (source ResearchSource) validate(effective, cutoff time.Time) error {
	if (source.Kind != ResearchSourceOperator && source.Kind != ResearchSourceProvider) ||
		!validResearchReference(source.Reference, 512) || source.AvailableAt.IsZero() {
		return errors.New("research source kind, reference and availability are required")
	}
	if source.AvailableAt.Before(effective) || source.AvailableAt.After(cutoff) {
		return errors.New("research source availability must be between the observation and research cutoff")
	}
	return nil
}

// AssessResearchAccount verifies the observed account and independent policy
// against a single research cutoff. The resulting view is detached from input
// memory and suitable for research consumers. It carries no execution rights.
func AssessResearchAccount(snapshot ResearchAccountSnapshot, policy *ResearchRiskPolicy, cutoff time.Time, maxAge time.Duration) (ResearchAccountView, error) {
	book := &snapshot.Book
	if err := ValidateAt(book, cutoff, maxAge); err != nil {
		return ResearchAccountView{}, err
	}
	view := ResearchAccountView{
		SchemaVersion: ResearchAccountSchemaVersion, RequestedAsOf: cutoff.UTC(),
		State: book.State, AccountAsOf: book.AsOf,
	}
	if book.State == StateNotProvided {
		if snapshot.AccountID != "" || snapshot.Version != "" || snapshot.Source != nil || len(snapshot.Costs) != 0 {
			return ResearchAccountView{}, errors.New("unknown account cannot carry observed account metadata")
		}
	} else {
		if !validResearchReference(snapshot.AccountID, 128) || !validResearchReference(snapshot.Version, 128) || snapshot.Source == nil {
			return ResearchAccountView{}, errors.New("observed account requires account ID, version and source")
		}
		effective, _ := time.Parse(time.RFC3339, book.AsOf)
		if err := snapshot.Source.validate(effective, cutoff); err != nil {
			return ResearchAccountView{}, fmt.Errorf("account provenance: %w", err)
		}
		source := *snapshot.Source
		view.AccountID, view.AccountVersion, view.AccountSource = snapshot.AccountID, snapshot.Version, &source
	}
	positions, err := researchPositions(book.Positions, snapshot.Costs)
	if err != nil {
		return ResearchAccountView{}, err
	}
	view.Positions = positions
	view.Exposure, err = ComputeExposure(book)
	if err != nil {
		return ResearchAccountView{}, err
	}
	if policy == nil {
		return view, nil
	}
	if !validResearchReference(policy.Version, 128) || (policy.LossBudgetFen != nil && *policy.LossBudgetFen < 0) {
		return ResearchAccountView{}, errors.New("invalid policy version or loss budget")
	}
	review, err := EvaluateCurrentLimits(book, &policy.Limits, cutoff)
	if err != nil {
		return ResearchAccountView{}, err
	}
	effective, _ := time.Parse(time.RFC3339, policy.Limits.AsOf)
	if err := policy.Source.validate(effective, cutoff); err != nil {
		return ResearchAccountView{}, fmt.Errorf("policy provenance: %w", err)
	}
	source := policy.Source
	view.PolicyVersion, view.PolicySource, view.LimitReview = policy.Version, &source, review
	if policy.LossBudgetFen != nil {
		amount := *policy.LossBudgetFen
		view.LossBudgetFen = &amount
	}
	return view, nil
}

func researchPositions(positions []Position, costs []PositionCost) ([]ResearchPosition, error) {
	result := make([]ResearchPosition, len(positions))
	indices := make(map[string]int, len(positions))
	for i, position := range positions {
		if strings.TrimSpace(position.Symbol) != position.Symbol {
			return nil, errors.New("observed position symbol must be canonical")
		}
		result[i].Position = position
		indices[position.Symbol] = i
	}
	for _, cost := range costs {
		i, ok := indices[cost.Symbol]
		if !ok || result[i].CostFen != nil || cost.CostFen == nil || *cost.CostFen < 0 {
			return nil, errors.New("position cost must match a unique holding and provide a nonnegative amount")
		}
		amount := *cost.CostFen
		result[i].CostFen = &amount
	}
	return result, nil
}
