package portfolio

import (
	"strings"
	"testing"
	"time"
)

func TestValidateAtPortfolioFreshness(t *testing.T) {
	requested := time.Date(2026, 10, 8, 13, 0, 0, 0, time.FixedZone("JST", 9*3600))
	// Same instant with different offsets must be compared as timestamps.
	tests := []struct {
		name string
		book *Context
		maxAge time.Duration
		want string
	}{
		{
			name:"same instant different timezone",
			book:&Context{State:StateEmpty,AsOf:"2026-10-08T12:00:00+08:00"},
			maxAge:time.Hour,
		},
		{
			name:"on exact freshness boundary",
			book:&Context{State:StatePositions,AsOf:"2026-10-08T10:00:00+08:00",
				Positions:[]Position{{Symbol:"600519.SH",Quantity:100}}},
			maxAge:2*time.Hour,
		},
		{
			name:"stale observed empty portfolio",
			book:&Context{State:StateEmpty,AsOf:"2026-10-07T10:00:00+08:00"},
			maxAge:24*time.Hour, want:"portfolio snapshot is stale",
		},
		{
			name:"stale observed positions",
			book:&Context{State:StatePositions,AsOf:"2026-10-07T10:00:00+08:00",
				Positions:[]Position{{Symbol:"600519.SH",Quantity:100}}},
			maxAge:24*time.Hour, want:"portfolio snapshot is stale",
		},
		{
			name:"future snapshot",
			book:&Context{State:StateEmpty,AsOf:"2026-10-08T12:00:01+08:00"},
			maxAge:24*time.Hour, want:"after research requested_as_of",
		},
		{
			name:"no assumed freshness for missing data",
			book:&Context{State:StateNotProvided},
			maxAge:time.Hour,
		},
		{
			name:"backward compatible when no age policy",
			book:&Context{State:StateEmpty,AsOf:"2020-01-01T00:00:00+08:00"},
			maxAge:0,
		},
		{
			name:"negative age not a disabled policy",
			book:&Context{State:StateNotProvided},
			maxAge:-time.Second, want:"portfolio max age must not be negative",
		},
	}
	for _,tc := range tests {
		t.Run(tc.name,func(t *testing.T){
			err:=ValidateAt(tc.book,requested,tc.maxAge)
			if tc.want=="" {
				if err!=nil {t.Fatalf("unexpected validation failure: %v",err)}
				return
			}
			if err==nil || !strings.Contains(err.Error(),tc.want) {
				t.Fatalf("error=%v, want %q",err,tc.want)
			}
		})
	}
}

func TestValidateAtRejectsZeroResearchCutoff(t *testing.T){
	err:=ValidateAt(&Context{State:StateNotProvided},time.Time{},0)
	if err==nil || !strings.Contains(err.Error(),"research requested_as_of") {
		t.Fatalf("error=%v",err)
	}
}
