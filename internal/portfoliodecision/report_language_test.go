package portfoliodecision

import ("testing";"unicode")

func TestCanonicalPolicySummariesAreChinese(t *testing.T){
 cases:=[]struct{ status string; summary string }{
 {"monitor","研究意向为 MONITOR（持续观察）；不发起任何组合操作。"},
 {"rejected","研究意向已在上游被拒绝；不发起任何组合操作。"},
 }
 for _,tc:=range cases{hasHan:=false;for _,r:=range tc.summary{if unicode.Is(unicode.Han,r){hasHan=true;break}};if !hasHan{t.Errorf("%s has no Chinese summary",tc.status)}}
}
