package application

import (
 "strings"
 "testing"
)

// Keep the agent's routing contract aligned with the read-only Eino tool
// registry: missing instructions can cause avoidable hallucinated comparisons.
func TestConversationInstructionGatesComparisonAndEvidence(t *testing.T) {
 for _,phrase:=range []string{
  "CompareResearchRuns", "ReadEvidenceIndex", "run_id",
  "研究截止时间", "不可比", "不从结论差异推断因果",
  "不保证原始来源真实性", "不得伪造",
 } {
  if !strings.Contains(conversationInstruction,phrase) {
   t.Fatalf("conversation instruction missing required guard: %q",phrase)
  }
 }
 if !strings.Contains(conversationInstruction,"StartResearch") ||
  !strings.Contains(conversationInstruction,"服务端授权") {
  t.Fatal("paid research start guard missing")
 }
}
