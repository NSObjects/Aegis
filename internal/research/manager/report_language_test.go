package manager

import ("strings";"testing")

func TestResearchManagerRequiresChineseNarrative(t *testing.T) {
 if !strings.Contains(researchManagerInstruction,"简体中文"){t.Fatal("research manager prose language contract missing")}
 if !strings.Contains(researchManagerInstruction,"accepted_argument_ids"){t.Fatal("must preserve argument reference schema")}
}
