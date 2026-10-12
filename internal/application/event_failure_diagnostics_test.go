package application

import (
 "errors"
 "strings"
 "testing"
)

func TestEventResearchErrorCategories(t *testing.T) {
 tests := []struct{name, message, want string}{
  {"json", "event research: event synthesis model: decode strict JSON after 2 attempts: invalid character", "EVENT_JSON_DECODE"},
  {"model", "event research: event synthesis model: generate model output: upstream unavailable", "EVENT_MODEL_GENERATE"},
  {"empty", "event research: event synthesis model: model returned empty output", "EVENT_MODEL_EMPTY"},
  {"context", "invalid event context: missing requested_as_of", "EVENT_CONTEXT_VALIDATION"},
  {"synthesis", "event research: normalize event analyst synthesis: unknown event ID", "EVENT_SYNTHESIS_VALIDATION"},
  {"unclassified", "event research: provider call failed with opaque status", "UNKNOWN"},
  {"otherstage", "market research: generate model output", "UNKNOWN"},
  {"secret", "external provider SECRET_API_KEY=12345 denied", "UNKNOWN"},
 }
 for _, tc := range tests {
  t.Run(tc.name,func(t *testing.T) {
   got:=researchErrorCategory(errors.New(tc.message))
   if got!=tc.want {t.Fatalf("got %q, want %q",got,tc.want)}
   if strings.Contains(got,"SECRET") || strings.Contains(got,"12345") {t.Fatal("category leaked provider details")}
  })
 }
}
