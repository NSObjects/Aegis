package application

import (
 "strings"
 "testing"
)

func TestConversationSessionIDRandomAndStrict(t *testing.T){
 seen:=make(map[string]bool)
 for i:=0;i<64;i++{
  id,err:=NewConversationSessionID()
  if err!=nil{t.Fatal(err)}
  if err:=ValidateConversationSessionID(id);err!=nil{t.Fatalf("generated invalid ID %q: %v",id,err)}
  if seen[id]{t.Fatal("duplicate generated session ID")}
  seen[id]=true
 }
 for _,id:=range []string{"",strings.Repeat("a",31),strings.Repeat("A",32),strings.Repeat("z",32),strings.Repeat("a",33),"../"+strings.Repeat("a",29)}{
  if ValidateConversationSessionID(id)==nil{t.Fatalf("accepted unsafe session ID: %q",id)}
 }
}
