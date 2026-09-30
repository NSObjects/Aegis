package event

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeImpactMechanismsGroundsAndSorts(t *testing.T) {
	facts:=[]Fact{
		groundedEvolutionFact("fact_contract","doc_contract","公司签订50亿元合同。"),
		groundedEvolutionFact("fact_capacity","doc_capacity","项目将分期交付。"),
	}
	got,err:=normalizeImpactMechanisms([]DetectedImpactMechanism{
		{Channel:ImpactRevenue,Direction:DirectionPositive,SupportingFactIDs:[]string{"fact_capacity","fact_contract"},Rationale:"contract creates potential future revenue",Uncertainty:"recognition depends on delivery"},
		{Channel:ImpactCashFlow,Direction:DirectionUncertain,SupportingFactIDs:[]string{"fact_contract"},Rationale:"cash timing is not specified"},
	},facts)
	if err!=nil { t.Fatal(err) }
	if len(got)!=2 { t.Fatalf("mechanisms=%+v",got) }
	if got[0].Channel!=ImpactCashFlow || got[1].Channel!=ImpactRevenue { t.Fatalf("order=%+v",got) }
	if !reflect.DeepEqual(got[1].SupportingFactIDs,[]string{"fact_capacity","fact_contract"}) { t.Fatalf("support=%v",got[1].SupportingFactIDs) }
	if got[1].ID!=deterministicImpactMechanismID(ImpactRevenue,DirectionPositive,got[1].SupportingFactIDs) { t.Fatalf("id=%s",got[1].ID) }
}

func TestNormalizeImpactMechanismsDeduplicatesDeterministically(t *testing.T) {
	facts:=[]Fact{groundedEvolutionFact("fact_a","doc_a","订单增加。")}
	got,err:=normalizeImpactMechanisms([]DetectedImpactMechanism{
		{Channel:ImpactRevenue,Direction:DirectionPositive,SupportingFactIDs:[]string{"fact_a"},Rationale:"revenue opportunity",Uncertainty:"timing unknown"},
		{Channel:ImpactRevenue,Direction:DirectionPositive,SupportingFactIDs:[]string{"fact_a","fact_a"},Rationale:"potential revenue",Uncertainty:""},
	},facts)
	if err!=nil { t.Fatal(err) }
	if len(got)!=1 { t.Fatalf("mechanisms=%+v",got) }
	if got[0].Rationale!="potential revenue" || got[0].Uncertainty!="timing unknown" { t.Fatalf("mechanism=%+v",got[0]) }
}

func TestNormalizeImpactMechanismsRejectsInvalidOutput(t *testing.T) {
	facts:=[]Fact{groundedEvolutionFact("fact_a","doc_a","事实A")}
	cases:=[]struct{name string; item DetectedImpactMechanism; want string}{
		{"channel",DetectedImpactMechanism{Channel:"STOCK_PRICE",Direction:DirectionPositive,SupportingFactIDs:[]string{"fact_a"},Rationale:"x"},"invalid channel"},
		{"direction",DetectedImpactMechanism{Channel:ImpactRevenue,Direction:"BUY",SupportingFactIDs:[]string{"fact_a"},Rationale:"x"},"invalid direction"},
		{"support",DetectedImpactMechanism{Channel:ImpactRevenue,Direction:DirectionPositive,Rationale:"x"},"supporting fact ids"},
		{"outside",DetectedImpactMechanism{Channel:ImpactRevenue,Direction:DirectionPositive,SupportingFactIDs:[]string{"missing"},Rationale:"x"},"outside the event cluster"},
		{"rationale",DetectedImpactMechanism{Channel:ImpactRevenue,Direction:DirectionPositive,SupportingFactIDs:[]string{"fact_a"}},"rationale is required"},
	}
	for _,tc:=range cases { t.Run(tc.name,func(t *testing.T){
		_,err:=normalizeImpactMechanisms([]DetectedImpactMechanism{tc.item},facts)
		if err==nil || !strings.Contains(err.Error(),tc.want) { t.Fatalf("error=%v",err) }
	})}
}

func TestNormalizeImpactMechanismsAllowsExplicitEmptyResult(t *testing.T) {
	facts:=[]Fact{groundedEvolutionFact("fact_a","doc_a","董事会完成例行换届。")}
	got,err:=normalizeImpactMechanisms(nil,facts)
	if err!=nil { t.Fatal(err) }
	if len(got)!=0 { t.Fatalf("mechanisms=%+v",got) }
}
