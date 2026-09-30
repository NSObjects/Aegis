package synthesis

import (
	"Aegis/internal/agent/event"
	capevent "Aegis/internal/capability/event"
	"Aegis/internal/agent/market"
	"Aegis/internal/agent/technical"
	"strings"
	"testing"
)

func TestValidateResearchArtifact(t *testing.T) {
	input := validSynthesisInput()

	artifact := &Artifact{
		Symbol:    input.Symbol,
		Market:    input.Market,
		Technical: input.Technical,
		Event:     input.Event,
		Synthesis: Synthesis{
			Summary: "市场偏弱，但公司事件提供独立的正向基本事实，技术面仍需确认。",
			Agreements: []Agreement{
				{
					Claim: "市场与技术上下文均提示短期风险仍需控制。",
					References: []Reference{
						{Domain: DomainMarket},
						{Domain: DomainTechnical},
					},
				},
			},
			Conflicts: []Conflict{
				{
					Description: "事件影响偏正向，但技术面仍弱。",
					References: []Reference{
						{Domain: DomainTechnical},
						{
							Domain:  DomainEvent,
							EventID: "event_contract",
						},
					},
				},
			},
			Risks: []string{
				"事件影响尚未被技术走势确认",
			},
			Confidence: 0.77,
		},
	}

	if err := Validate(artifact); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateInputPreservesExistingTemporalBoundary(
	t *testing.T,
) {
	input := validSynthesisInput()

	// Market/technical use trading-date strings while EventContext uses an
	// RFC3339 timestamp. V2.7.13 intentionally does not pretend they share
	// identical precision.
	if err := ValidateInput(input); err != nil {
		t.Fatalf(
			"ValidateInput() mixed time granularity error = %v",
			err,
		)
	}

	input.Technical.RequestedAsOf = "20260930"
	if err := ValidateInput(input); err != nil {
		t.Fatalf("older market effective date should be PIT-safe: %v", err)
	}

	input.Technical.RequestedAsOf = "20260928"
	err := ValidateInput(input)
	if err == nil || !strings.Contains(err.Error(), "is after technical requested_as_of") {
		t.Fatalf("error = %v, want future market data rejection", err)
	}
}

func TestValidateInputRejectsSymbolMismatch(t *testing.T) {
	input := validSynthesisInput()
	input.Event.Symbol = "000001.SZ"

	err := ValidateInput(input)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"event context symbol",
		) {
		t.Fatalf(
			"error = %v, want event-symbol mismatch",
			err,
		)
	}
}

func TestValidateSynthesisReferenceInvariants(
	t *testing.T,
) {
	eventContext := validSynthesisInput().Event

	tests := []struct {
		name      string
		synthesis Synthesis
		want      string
	}{
		{
			name: "agreement without reference",
			synthesis: Synthesis{
				Summary: "summary",
				Agreements: []Agreement{
					{Claim: "claim"},
				},
				Confidence: 0.5,
			},
			want: "requires at least one reference",
		},
		{
			name: "conflict one domain",
			synthesis: Synthesis{
				Summary: "summary",
				Conflicts: []Conflict{
					{
						Description: "conflict",
						References: []Reference{
							{Domain: DomainMarket},
							{Domain: DomainMarket},
						},
					},
				},
				Confidence: 0.5,
			},
			want: "at least two distinct domains",
		},
		{
			name: "unknown event id",
			synthesis: Synthesis{
				Summary: "summary",
				Agreements: []Agreement{
					{
						Claim: "claim",
						References: []Reference{
							{
								Domain:  DomainEvent,
								EventID: "event_missing",
							},
						},
					},
				},
				Confidence: 0.5,
			},
			want: "does not exist in EventContext",
		},
		{
			name: "market carries event id",
			synthesis: Synthesis{
				Summary: "summary",
				Agreements: []Agreement{
					{
						Claim: "claim",
						References: []Reference{
							{
								Domain:  DomainMarket,
								EventID: "event_contract",
							},
						},
					},
				},
				Confidence: 0.5,
			},
			want: "cannot carry event_id",
		},
		{
			name: "invalid domain",
			synthesis: Synthesis{
				Summary: "summary",
				Agreements: []Agreement{
					{
						Claim: "claim",
						References: []Reference{
							{Domain: Domain("FUNDAMENTAL")},
						},
					},
				},
				Confidence: 0.5,
			},
			want: "invalid synthesis reference domain",
		},
		{
			name: "invalid confidence",
			synthesis: Synthesis{
				Summary:    "summary",
				Confidence: 1.2,
			},
			want: "confidence out of range",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateSynthesis(
				test.synthesis,
				eventContext,
			)
			if err == nil ||
				!strings.Contains(
					err.Error(),
					test.want,
				) {
				t.Fatalf(
					"error = %v, want substring %q",
					err,
					test.want,
				)
			}
		})
	}
}

func validSynthesisInput() Input {
	return Input{
		Symbol: "600519.SH",
		Market: &market.Context{
			AsOf:       "20260929",
			Regime:     market.RegimeMixed,
			Evidence:   []string{"市场分化"},
			Risk:       []string{"风险偏好一般"},
			Confidence: 0.70,
		},
		Technical: &technical.Context{
			Symbol:        "600519.SH",
			RequestedAsOf: "20260929",
			EffectiveAsOf: "20260929",
			Status:        technical.StatusNormal,
			Snapshot: &technical.Snapshot{
				Close: 1500,
			},
			Evidence:   []string{"价格位于中期均线附近"},
			Risk:       []string{"趋势确认不足"},
			Confidence: 0.75,
		},
		Event: &event.Context{
			Symbol:        "600519.SH",
			RequestedAsOf: "2026-09-29T15:00:00Z",
			Status:        event.StatusNormal,
			Events: []event.Event{
				{
					ID:          "event_contract",
					Type:        event.EventContract,
					OccurredAt:  "2026-09-29T09:00:00Z",
					Headline:    "合同事件",
					FactSummary: "公司披露签订合同。",
					Facts: []event.Fact{
						{
							ID:       "fact_contract",
							Type:     event.EventContract,
							Headline: "合同事件",
							Summary:  "公司披露签订合同。",
							Groundings: []event.EvidenceGrounding{
								{
									DocumentID: "doc_1",
									Quote:      "公司披露签订合同。",
									StartByte:  0,
									EndByte: len(
										"公司披露签订合同。",
									),
								},
							},
							EvidenceDocumentIDs: []string{
								"doc_1",
							},
							Confidence: 0.95,
						},
					},
					EvidenceDocumentIDs: []string{
						"doc_1",
					},
					SourceCount:      1,
					HasPrimarySource: true,
					EvidenceSupport: capevent.EvidenceSupport{
						DocumentCount:            1,
						IndependentEvidenceCount: 1,
						IndependentSourceCount:   1,
						PrimarySourcePresent:     true,
						SupportClass: capevent.
							EvidenceSupportPrimaryOnly,
					},
					FactConfidence: 0.95,
					Materiality: event.Materiality{
						Level:      event.MaterialityMedium,
						Direction:  event.DirectionPositive,
						Horizon:    event.HorizonShortTerm,
						Rationale:  "合同可能影响后续收入。",
						Confidence: 0.78,
					},
				},
			},
			Evidence:   []string{"doc_1"},
			Risk:       []string{"履约存在不确定性"},
			Confidence: 0.84,
		},
	}
}
