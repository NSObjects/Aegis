package application

import (
 "context"
 "path/filepath"
 "testing"
 "time"

 technicalcap "Aegis/internal/capability/technical"
)

func TestHistoricalOHLCVPersistsAndReopens(t *testing.T) {
 dir:=filepath.Join(t.TempDir(),"runs")
 store,err:=NewFileRunStore(dir);if err!=nil{t.Fatal(err)}
 record:=RunRecord{ID:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",Mode:ModeDecision,Symbol:"600519.SH",RequestedAsOf:time.Date(2026,10,12,7,0,0,0,time.UTC),CreatedAt:time.Now().UTC(),State:RunRunning}
 record.HistoricalOHLCV=&technicalcap.HistoricalObservation{Symbol:"600519.SH",RequestedAsOf:"20261012",EffectiveAsOf:"20261009",PriceBasis:"RAW",Bars:[]technicalcap.HistoricalBar{{TradeDate:"20261009",Open:10,High:12,Low:9,Close:11,Volume:100,Amount:1000}}}
 if err:=store.Save(context.Background(),record);err!=nil{t.Fatal(err)}
 fresh,err:=NewFileRunStore(dir);if err!=nil{t.Fatal(err)}
 got,err:=fresh.Get(context.Background(),record.ID);if err!=nil{t.Fatal(err)}
 if got.HistoricalOHLCV==nil||len(got.HistoricalOHLCV.Bars)!=1||got.HistoricalOHLCV.Bars[0].Close!=11{t.Fatalf("history lost after reopen: %#v",got.HistoricalOHLCV)}
 record.HistoricalOHLCV.Bars[0].Close=999
 again,err:=fresh.Get(context.Background(),record.ID);if err!=nil{t.Fatal(err)}
 if again.HistoricalOHLCV.Bars[0].Close!=11{t.Fatal("persisted source mutated through caller")}
}

func TestHistoricalOHLCVRejectsWrongIdentityFutureAndDuplicates(t *testing.T) {
 record:=RunRecord{Mode:ModeDecision,Symbol:"600519.SH",State:RunRunning,RequestedAsOf:time.Date(2026,10,12,7,0,0,0,time.UTC)}
 good:=technicalcap.HistoricalBar{TradeDate:"20261009",Open:10,High:12,Low:9,Close:11,Volume:100}
 base:=technicalcap.HistoricalObservation{Symbol:"600519.SH",RequestedAsOf:"20261012",EffectiveAsOf:"20261009",PriceBasis:"RAW",Bars:[]technicalcap.HistoricalBar{good}}
 if err:=validateHistoricalObservation(record,&base);err!=nil{t.Fatal(err)}
 cases:=[]struct{name string; mutate func(*technicalcap.HistoricalObservation)}{
  {"symbol",func(o *technicalcap.HistoricalObservation){o.Symbol="000001.SZ"}},
  {"future",func(o *technicalcap.HistoricalObservation){o.Bars[0].TradeDate="20261013";o.EffectiveAsOf="20261013"}},
  {"duplicate",func(o *technicalcap.HistoricalObservation){o.Bars=append(o.Bars,good)}},
  {"basis",func(o *technicalcap.HistoricalObservation){o.PriceBasis="QFQ"}},
  {"impossible",func(o *technicalcap.HistoricalObservation){o.Bars[0].High=9}},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){c:=base;c.Bars=append([]technicalcap.HistoricalBar(nil),base.Bars...);tc.mutate(&c);if err:=validateHistoricalObservation(record,&c);err==nil{t.Fatal("invalid source accepted")}})}
}
