package technical

import "testing"

func TestValidate(t *testing.T) {
	value := func(v float64) *float64 {
		return &v
	}

	validNormal := func() *Context {
		return &Context{
			Symbol:        "600519.SH",
			RequestedAsOf: "20260923",
			EffectiveAsOf: "20260923",
			Status:        StatusNormal,
			Snapshot: &Snapshot{
				Open:          1250,
				High:          1260,
				Low:           1248,
				Close:         1251.24,
				PreClose:      1253.80,
				ChangePercent: -0.20,
				Volume:        3_000_000,
				Amount:        3_800_000_000,
			},
			Returns: Returns{
				Return5D: value(-0.54),
			},
			MovingAverages: MovingAverages{
				MA5: value(1256.3),
			},
			Momentum: Momentum{
				RSI14: value(42.1),
				MACD: MACD{
					DIF: value(-8.2),
					DEA: value(-6.9),
				},
			},
			Evidence: []string{
				"close below MA5",
			},
			Risk: []string{
				"short-term rebound possible",
			},
			Confidence: 0.7,
		}
	}

	t.Run("normal", func(t *testing.T) {
		if err := Validate(validNormal()); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})

	t.Run("suspended", func(t *testing.T) {
		ctx := &Context{
			Symbol:        "600519.SH",
			RequestedAsOf: "20260923",
			LastTradeDate: "20260922",
			Status:        StatusSuspended,
			Risk: []string{
				"stock is suspended",
			},
			Confidence: 0.3,
		}

		if err := Validate(ctx); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})

	t.Run("no data", func(t *testing.T) {
		ctx := &Context{
			Symbol:        "600519.SH",
			RequestedAsOf: "20260923",
			Status:        StatusNoData,
			Risk: []string{
				"market data unavailable",
			},
			Confidence: 0,
		}

		if err := Validate(ctx); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})

	tests := []struct {
		name string
		ctx  *Context
	}{
		{
			name: "nil",
			ctx:  nil,
		},
		{
			name: "missing symbol",
			ctx: func() *Context {
				v := validNormal()
				v.Symbol = ""
				return v
			}(),
		},
		{
			name: "missing requested as of",
			ctx: func() *Context {
				v := validNormal()
				v.RequestedAsOf = ""
				return v
			}(),
		},
		{
			name: "normal missing effective as of",
			ctx: func() *Context {
				v := validNormal()
				v.EffectiveAsOf = ""
				return v
			}(),
		},
		{
			name: "normal missing snapshot",
			ctx: func() *Context {
				v := validNormal()
				v.Snapshot = nil
				return v
			}(),
		},
		{
			name: "normal missing evidence",
			ctx: func() *Context {
				v := validNormal()
				v.Evidence = nil
				return v
			}(),
		},
		{
			name: "suspended missing last trade date",
			ctx: &Context{
				Symbol:        "600519.SH",
				RequestedAsOf: "20260923",
				Status:        StatusSuspended,
				Risk:          []string{"suspended"},
			},
		},
		{
			name: "invalid status",
			ctx: func() *Context {
				v := validNormal()
				v.Status = Status("UNKNOWN")
				return v
			}(),
		},
		{
			name: "confidence below range",
			ctx: func() *Context {
				v := validNormal()
				v.Confidence = -0.1
				return v
			}(),
		},
		{
			name: "confidence above range",
			ctx: func() *Context {
				v := validNormal()
				v.Confidence = 1.1
				return v
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Validate(tt.ctx); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
