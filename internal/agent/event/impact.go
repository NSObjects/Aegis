package event

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type ImpactChannel string

const (
	ImpactRevenue    ImpactChannel = "REVENUE"
	ImpactProfit     ImpactChannel = "PROFIT"
	ImpactCashFlow   ImpactChannel = "CASH_FLOW"
	ImpactCost       ImpactChannel = "COST"
	ImpactCapacity   ImpactChannel = "CAPACITY"
	ImpactDemand     ImpactChannel = "DEMAND"
	ImpactSupply     ImpactChannel = "SUPPLY"
	ImpactFinancing  ImpactChannel = "FINANCING"
	ImpactOwnership  ImpactChannel = "OWNERSHIP"
	ImpactRegulatory ImpactChannel = "REGULATORY"
	ImpactOperations ImpactChannel = "OPERATIONS"
	ImpactReputation ImpactChannel = "REPUTATION"
)

type ImpactDirection string

const (
	ImpactPositive  ImpactDirection = "POSITIVE"
	ImpactNegative  ImpactDirection = "NEGATIVE"
	ImpactMixed     ImpactDirection = "MIXED"
	ImpactNeutral   ImpactDirection = "NEUTRAL"
	ImpactUncertain ImpactDirection = "UNCERTAIN"
)

type DetectedImpactMechanism struct {
	Channel           ImpactChannel   `json:"channel"`
	Direction         ImpactDirection `json:"direction"`
	SupportingFactIDs []string        `json:"supporting_fact_ids"`
	Rationale         string          `json:"rationale"`
	Uncertainty       string          `json:"uncertainty,omitempty"`
}

type ImpactMechanismDetectionResult struct {
	Mechanisms []DetectedImpactMechanism `json:"mechanisms"`
}

type ImpactMechanism struct {
	ID                string          `json:"id"`
	Channel           ImpactChannel   `json:"channel"`
	Direction         ImpactDirection `json:"direction"`
	SupportingFactIDs []string        `json:"supporting_fact_ids"`
	Rationale         string          `json:"rationale"`
	Uncertainty       string          `json:"uncertainty,omitempty"`
}

func normalizeImpactMechanisms(
	detected []DetectedImpactMechanism,
	facts []Fact,
) ([]ImpactMechanism, error) {
	factIDs := make(map[string]struct{}, len(facts))
	for _, fact := range facts {
		if strings.TrimSpace(fact.ID) == "" || len(fact.Groundings) == 0 {
			return nil, errors.New("impact mechanism detection requires grounded facts")
		}
		factIDs[fact.ID] = struct{}{}
	}

	seen := make(map[string]int, len(detected))
	result := make([]ImpactMechanism, 0, len(detected))
	for index, raw := range detected {
		if !validImpactChannel(raw.Channel) {
			return nil, fmt.Errorf("impact mechanism %d has invalid channel %q", index, raw.Channel)
		}
		if !validImpactDirection(raw.Direction) {
			return nil, fmt.Errorf("impact mechanism %d has invalid direction %q", index, raw.Direction)
		}
		rationale := strings.TrimSpace(raw.Rationale)
		if rationale == "" {
			return nil, fmt.Errorf("impact mechanism %d rationale is required", index)
		}
		supporting := uniqueSortedStrings(raw.SupportingFactIDs)
		if len(supporting) == 0 {
			return nil, fmt.Errorf("impact mechanism %d requires supporting fact ids", index)
		}
		for _, factID := range supporting {
			if _, ok := factIDs[factID]; !ok {
				return nil, fmt.Errorf("impact mechanism %d references fact outside the event cluster", index)
			}
		}
		uncertainty := strings.TrimSpace(raw.Uncertainty)
		key := string(raw.Channel)+"\x00"+string(raw.Direction)+"\x00"+strings.Join(supporting,"\x00")
		if existing, duplicate := seen[key]; duplicate {
			if rationale < result[existing].Rationale { result[existing].Rationale = rationale }
			if uncertainty != "" && (result[existing].Uncertainty == "" || uncertainty < result[existing].Uncertainty) {
				result[existing].Uncertainty = uncertainty
			}
			continue
		}
		seen[key] = len(result)
		result = append(result, ImpactMechanism{
			ID: deterministicImpactMechanismID(raw.Channel, raw.Direction, supporting),
			Channel:raw.Channel, Direction:raw.Direction, SupportingFactIDs:supporting,
			Rationale:rationale, Uncertainty:uncertainty,
		})
	}
	sort.SliceStable(result, func(i,j int) bool {
		if result[i].Channel != result[j].Channel { return result[i].Channel < result[j].Channel }
		if result[i].Direction != result[j].Direction { return result[i].Direction < result[j].Direction }
		return strings.Join(result[i].SupportingFactIDs,"\x00") < strings.Join(result[j].SupportingFactIDs,"\x00")
	})
	return result,nil
}

func deterministicImpactMechanismID(channel ImpactChannel, direction ImpactDirection, factIDs []string) string {
	ids:=append([]string(nil),factIDs...)
	sort.Strings(ids)
	sum:=sha256.Sum256([]byte(string(channel)+"\x00"+string(direction)+"\x00"+strings.Join(ids,"\x00")))
	return "impact_"+hex.EncodeToString(sum[:8])
}

func validImpactChannel(v ImpactChannel) bool {
	switch v {
	case ImpactRevenue,ImpactProfit,ImpactCashFlow,ImpactCost,ImpactCapacity,ImpactDemand,
		ImpactSupply,ImpactFinancing,ImpactOwnership,ImpactRegulatory,ImpactOperations,ImpactReputation:
		return true
	default:return false
	}
}

func validImpactDirection(v ImpactDirection) bool {
	switch v {
	case ImpactPositive,ImpactNegative,ImpactMixed,ImpactNeutral,ImpactUncertain:return true
	default:return false
	}
}
