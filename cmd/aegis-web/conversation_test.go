package main

import (
 "context"
 "strings"
 "testing"
)

func TestConversationRuntimeRejectsIncompleteConfiguration(t *testing.T) {
 cases:=[]*conversationRuntime{nil,{}, {configPath:"configs/config.yaml"}}
 for _,runtime:=range cases {
  if _,err:=runtime.Respond(context.Background(),"查询已有研究");err==nil{t.Fatal("incomplete runtime unexpectedly accepted")}
 }
}

func TestConversationRuntimeDoesNotAcceptNilContext(t *testing.T) {
 runtime:=&conversationRuntime{}
 if _,err:=runtime.Respond(nil,strings.Repeat("a",12));err==nil{t.Fatal("nil context accepted")}
}
