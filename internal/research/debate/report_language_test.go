package debate

import ("strings";"testing")

func TestResearchDebateRequiresChineseNarrative(t *testing.T) {
 for name,prompt:=range map[string]string{"bull":bullInstruction,"bear":bearInstruction}{
  if !strings.Contains(prompt,"简体中文") {t.Fatalf("%s debate narrative must be simplified Chinese",name)}
  for _,literal:=range []string{`"side": "BULL"`,`"side": "BEAR"`}{
   if strings.Contains(prompt,literal) {break}
  }
 }
}
