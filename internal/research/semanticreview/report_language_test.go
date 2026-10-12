package semanticreview

import ("strings";"testing")

func TestSemanticReviewerChineseExplanation(t *testing.T) {
 if !strings.Contains(semanticReviewerInstruction,"Simplified Chinese"){t.Fatal("advisory reviewer text must be Chinese")}
 if !strings.Contains(semanticReviewerInstruction,"UNCERTAIN"){t.Fatal("verdict enums must remain stable")}
}
