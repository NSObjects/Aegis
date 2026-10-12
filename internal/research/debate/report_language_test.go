package debate

import (
 "strings"
 "testing"
)

func TestResearchDebateRequiresChineseNarrative(t *testing.T) {
 cases:=[]struct{name,prompt,side string}{
  {"bull",bullInstruction,`"side": "BULL"`},
  {"bear",bearInstruction,`"side": "BEAR"`},
 }
 for _,tc:=range cases {
  if !strings.Contains(tc.prompt,"简体中文") {
   t.Errorf("%s debate thesis, claims and rebuttals must be Chinese",tc.name)
  }
  if !strings.Contains(tc.prompt,tc.side) {
   t.Errorf("%s must retain exact typed side enum",tc.name)
  }
  if !strings.Contains(tc.prompt,"event_id") {
   t.Errorf("%s must preserve typed event citations",tc.name)
  }
 }
}
