package event

import ("strings";"testing")

func TestEventAnalystChineseAndVerbatimEvidence(t *testing.T){
 prompts:=map[string]string{
  "extract":factExtractionInstruction,
  "contradictions":contradictionDetectionInstruction,
  "evolution":factEvolutionInstruction,
  "mechanisms":impactMechanismInstruction,
  "summary":synthesisInstruction,
  "materiality":materialityInstruction,
 }
 for name,prompt:=range prompts {
  if !strings.Contains(prompt,"Simplified Chinese"){t.Errorf("%s narrative should be Chinese",name)}
 }
 if !strings.Contains(factExtractionInstruction,"VERBATIM"){t.Fatal("evidence quote must remain source-language verbatim")}
}
