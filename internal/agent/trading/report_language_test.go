package trading

import ("strings";"testing")

func TestDecisionOutputLanguage(t *testing.T){
 prompts:=[]string{instruction,artifactInstruction,researchPlanInstruction}
 for i,p:=range prompts{
  if !strings.Contains(p,"简体中文"){t.Errorf("trading prompt %d lacks Chinese output requirement",i)}
  if !strings.Contains(p,"CANDIDATE"){t.Errorf("trading prompt %d lost enum",i)}
 }
}
