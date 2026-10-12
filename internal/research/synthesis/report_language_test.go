package synthesis

import ("strings";"testing")

func TestResearchSynthesisRequiresChineseNarrative(t *testing.T) {
 if !strings.Contains(researchSynthesisInstruction,"简体中文"){t.Fatal("synthesis narrative must be simplified Chinese")}
 if !strings.Contains(researchSynthesisInstruction,`"summary"`){t.Fatal("typed synthesis schema missing")}
}
