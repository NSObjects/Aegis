package technical

import "testing"

func TestObservedHistoricalBars(t *testing.T) {
 bars:=[]bar{
  {Date:"20261009",Open:10,High:12,Low:9,Close:11,Vol:100,Amount:1000},
  {Date:"20261012",Open:11,High:13,Low:10,Close:12,Vol:110,Amount:1100},
 }
 out,err:=observedHistoricalBars(bars)
 if err!=nil{t.Fatal(err)}
 if len(out)!=2||out[0].TradeDate!="20261009"||out[1].Close!=12{t.Fatalf("unexpected bars %#v",out)}
 if out[0].Volume!=100{t.Fatal("source volume mutated")}
}

func TestObservedHistoricalBarsRejectsInvalidSequence(t *testing.T) {
 good:=bar{Date:"20261009",Open:10,High:12,Low:9,Close:11,Vol:100,Amount:1000}
 for _,test:=range []struct{name string; bars []bar}{
  {"duplicate",[]bar{good,good}},
  {"reverse",[]bar{{Date:"20261012",Open:10,High:12,Low:9,Close:11},good}},
  {"impossible",[]bar{{Date:"20261009",Open:10,High:9,Low:8,Close:11}}},
  {"negative_volume",[]bar{{Date:"20261009",Open:10,High:12,Low:9,Close:11,Vol:-1}}},
 } {
  t.Run(test.name,func(t *testing.T){if _,err:=observedHistoricalBars(test.bars);err==nil{t.Fatal("expected rejection")}})
 }
}
