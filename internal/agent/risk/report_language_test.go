package risk

import ("strings";"testing")

func TestRiskNarrativesUseChinese(t *testing.T) {
 if !strings.Contains(instruction,"简体中文"){t.Fatal("risk LLM prompt must request Chinese narratives")}
}
