package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func canaryTestNow() time.Time {
	return time.Date(2026,11,16,9,0,0,0,time.FixedZone("Asia/Shanghai",8*3600))
}
func TestCanaryDryRunNeverNeedsTokenOrNetwork(t *testing.T) {
	var output bytes.Buffer
	err := run(context.Background(),"20261113",3,false,"should-not-print-this",
		&output,canaryTestNow())
	if err != nil {t.Fatal(err)}
	var plan canaryPlan
	if err:=json.Unmarshal(output.Bytes(),&plan);err!=nil{t.Fatal(err)}
	if plan.NetworkEnabled || plan.MaxProviderCalls!=10 ||
		plan.AsOf!="20261113" || plan.Days!=3 {
		t.Fatalf("unexpected dry-run budget: %+v",plan)
	}
	if strings.Contains(output.String(),"should-not-print-this") {
		t.Fatal("credential was included in dry-run output")
	}
}

func TestCanaryRefusesCurrentIncompleteDayAndUnboundedInput(t *testing.T) {
	for _,tc:=range []struct{date string;days int}{
		{"20261116",1}, // same Shanghai date, even after 9am
		{"20261117",1},
		{"20261113",0},
		{"20261113",4},
		{"20260230",1},
		{"bad",1},
	} {
		var output bytes.Buffer
		if err:=run(context.Background(),tc.date,tc.days,false,"",&output,canaryTestNow());err==nil {
			t.Fatalf("invalid canary accepted: %+v",tc)
		}
		if output.Len()!=0 {t.Fatal("invalid canary emitted a report")}
	}
}

func TestCanaryExecuteRequiresToken(t *testing.T) {
	var output bytes.Buffer
	err:=run(context.Background(),"20261113",1,true,"",&output,canaryTestNow())
	if err==nil||!strings.Contains(err.Error(),"TUSHARE_TOKEN") {
		t.Fatalf("missing token was allowed: %v",err)
	}
	if output.Len()!=0 {t.Fatal("failed credential gate emitted a report")}
}
