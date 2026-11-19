package pullback

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

const strategyVersionV1 = "trend-pullback-v1"

// StrategyVersion is the default version for new scans.
const StrategyVersion = strategyVersionV1

const (
	v1TrendFastSessions = 20
	v1TrendSlowSessions = 60
	v1SlopeLagSessions  = 5
	v1CloseUpperRatio   = 1.03
)

// StrategyCandidate describes the immutable inputs to a registered rule.
// A change to the evaluation semantics or the parameters requires a new version.
type StrategyCandidate struct {
	Version               string  `json:"version"`
	Universe              string  `json:"universe"`
	PriceBasis            string  `json:"price_basis"`
	AdjustmentAnchor      string  `json:"adjustment_anchor"`
	FastSessions          int     `json:"fast_sessions"`
	SlowSessions          int     `json:"slow_sessions"`
	SlopeLagSessions      int     `json:"slope_lag_sessions"`
	CloseLowerRatio       float64 `json:"close_lower_ratio"`
	CloseUpperRatio       float64 `json:"close_upper_ratio"`
	RequirePositiveVolume bool    `json:"require_positive_volume"`
}

var ErrUnknownStrategy = errors.New("unsupported pullback strategy version")

func Definition(version string) (StrategyCandidate, error) {
	switch version {
	case strategyVersionV1:
		return StrategyCandidate{
			Version: strategyVersionV1, Universe: "ASHARE_MAIN_BOARD", PriceBasis: "RAW",
			AdjustmentAnchor: "PIT_FACTOR_AT_AS_OF", FastSessions: v1TrendFastSessions,
			SlowSessions: v1TrendSlowSessions, SlopeLagSessions: v1SlopeLagSessions,
			CloseLowerRatio: 1, CloseUpperRatio: v1CloseUpperRatio, RequirePositiveVolume: true,
		}, nil
	default:
		return StrategyCandidate{}, ErrUnknownStrategy
	}
}

func (candidate StrategyCandidate) SHA256() (string, error) {
	raw, err := json.Marshal(candidate)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
