package synthesis

import (
 "testing"
 "strings"
)

func TestFundamentalReferenceRequiresPresentContext(t *testing.T) {
 ref:=Reference{Domain:DomainFundamental}
 if err:=validateReference(ref,nil);err==nil||!strings.Contains(err.Error(),"requires FundamentalContext"){t.Fatalf("missing source must fail: %v",err)}
 if err:=validateReference(ref,nil,true);err!=nil{t.Fatalf("present source rejected: %v",err)}
 if err:=validateReference(Reference{Domain:DomainFundamental,EventID:"x"},nil,true);err==nil{t.Fatal("fundamental must not carry event_id")}
}
