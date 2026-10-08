package portfolio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Decoding a present valuation is deliberately strict: zero cents is a valid
// observation; an OMITTED field is not an observation and must not be silently
// converted to zero by Go's JSON decoder.
func (v *Valuation) UnmarshalJSON(data []byte) error {
	var raw struct {
		Scope          *string          `json:"scope"`
		Currency       *string          `json:"currency"`
		TotalValueFen  *int64           `json:"total_value_fen"`
		CashFen        *int64           `json:"cash_fen"`
		PositionValues *[]PositionValue `json:"position_values"`
	}
	if err := decodeStrictValuationJSON(data, &raw); err != nil {
		return fmt.Errorf("decode observed valuation: %w", err)
	}
	if raw.Scope == nil || raw.Currency == nil || raw.TotalValueFen == nil ||
		raw.CashFen == nil || raw.PositionValues == nil {
		return errors.New("valuation requires explicit scope, currency, total_value_fen, cash_fen and position_values (including zero values)")
	}
	*v = Valuation{
		Scope: *raw.Scope, Currency: *raw.Currency,
		TotalValueFen: *raw.TotalValueFen, CashFen: *raw.CashFen,
		PositionValues: *raw.PositionValues,
	}
	return nil
}

func (p *PositionValue) UnmarshalJSON(data []byte) error {
	var raw struct {
		Symbol         *string `json:"symbol"`
		MarketValueFen *int64  `json:"market_value_fen"`
	}
	if err := decodeStrictValuationJSON(data, &raw); err != nil {
		return fmt.Errorf("decode observed position value: %w", err)
	}
	if raw.Symbol == nil || raw.MarketValueFen == nil {
		return errors.New("observed position value requires explicit symbol and market_value_fen (including zero)")
	}
	*p = PositionValue{Symbol: *raw.Symbol, MarketValueFen: *raw.MarketValueFen}
	return nil
}

func decodeStrictValuationJSON(data []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}
