package event

import (
 "strings"
 "testing"
)

func TestValidateDetectedImpactEnums(t *testing.T) {
 tests:=[]struct{
  name string
  mechanisms []DetectedImpactMechanism
  want string
 }{
  {"valid",[]DetectedImpactMechanism{{Channel:ImpactRevenue,Direction:DirectionPositive}},""},
  {"market pricing is not a business channel",[]DetectedImpactMechanism{{Channel:ImpactChannel("PRICING"),Direction:DirectionPositive}},"invalid channel"},
  {"invalid direction",[]DetectedImpactMechanism{{Channel:ImpactRevenue,Direction:ImpactDirection("BUY")}},"invalid direction"},
 }
 for _,tt:=range tests{
  t.Run(tt.name,func(t *testing.T){
   err:=validateDetectedImpactEnums(tt.mechanisms)
   if tt.want==""{if err!=nil{t.Fatalf("unexpected: %v",err)};return}
   if err==nil||!strings.Contains(err.Error(),tt.want){t.Fatalf("got %v, want %q",err,tt.want)}
  })
 }
}
