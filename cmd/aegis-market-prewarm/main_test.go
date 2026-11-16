package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func frozenNow()time.Time{
	return time.Date(2026,11,16,16,0,0,0,time.FixedZone("Asia/Shanghai",8*3600))
}
func TestFullPrewarmPreviewHasNoNetworkOrToken(t *testing.T) {
	var stdout,stderr bytes.Buffer
	err:=run(context.Background(),"20261113",false,"must-not-appear",&stdout,&stderr,frozenNow())
	if err!=nil{t.Fatal(err)}
	var plan prewarmPlan
	if err:=json.Unmarshal(stdout.Bytes(),&plan);err!=nil{t.Fatal(err)}
	if plan.NetworkEnabled||plan.MaxProviderCalls!=184||plan.Sessions!=90||
		plan.AsOf!="20261113"||!strings.Contains(plan.Message,"PREVIEW ONLY") {
		t.Fatalf("invalid dry run plan: %+v",plan)
	}
	if strings.Contains(stdout.String(),"must-not-appear")||stderr.Len()!=0 {
		t.Fatal("preview exposed token or attempted progress")
	}
}
func TestFullPrewarmRejectsFutureDatesAndMissingLiveToken(t *testing.T) {
	for _,date:=range []string{"20261116","20261117","20261377","invalid"} {
		var out,errors bytes.Buffer
		if err:=run(context.Background(),date,false,"",&out,&errors,frozenNow());err==nil{
			t.Fatalf("invalid date accepted: %s",date)
		}
		if out.Len()!=0||errors.Len()!=0{t.Fatal("invalid date emitted unexpected output")}
	}
	var stdout,stderr bytes.Buffer
	err:=run(context.Background(),"20261113",true,"",&stdout,&stderr,frozenNow())
	if err==nil||!strings.Contains(err.Error(),"TUSHARE_TOKEN") {
		t.Fatalf("LIVE without token accepted: %v",err)
	}
	if stdout.Len()!=0||stderr.Len()!=0{t.Fatal("failed credential gate produced output")}
}
