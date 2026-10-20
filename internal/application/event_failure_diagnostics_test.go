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
  {"acquisition plan", "event research: run event evidence research loop: execute research acquisition plan: event evidence selector is required", "EVENT_ACQUISITION_PLAN"},
  {"round canonicalization", "event research: run event evidence research loop: canonicalize accumulated research evidence: malformed raw reference", "EVENT_ROUND_CANONICALIZATION"},
  {"plan compile", "event research: run event evidence research loop: compile research plan: unsupported channel", "EVENT_PLAN_COMPILE"},
  {"acquisition stage isolation", "market research: execute research acquisition plan: bad", "UNKNOWN"},
  {"evidence loop", "event research: run event evidence research loop: acquisition step failed", "EVENT_EVIDENCE_LOOP"},
  {"capture", "event research: capture live event evidence: canonicalize failed", "EVENT_CAPTURE"},
  {"analysis", "event research: analyze researched event evidence: unsupported quote", "EVENT_ANALYSIS"},
  {"fact extraction", "event research: analyze researched event evidence: extract event facts: provider output invalid", "EVENT_FACT_EXTRACTION"},
  {"fact clustering", "event research: analyze researched event evidence: cluster event facts: invalid group", "EVENT_FACT_CLUSTERING"},
  {"relations", "event research: analyze researched event evidence: detect event \"x\" contradictions: invalid", "EVENT_EVENT_RELATION"},
  {"impact", "event research: analyze researched event evidence: analyze event \"x\" impact mechanisms: invalid", "EVENT_IMPACT_OR_MATERIALITY"},
  {"normalize", "event research: analyze researched event evidence: normalize event \"x\" fact evolution: unsupported", "EVENT_ANALYSIS_NORMALIZATION"},
  {"synthesis", "event research: analyze researched event evidence: synthesize event analyst conclusion: generation failure", "EVENT_ANALYST_SYNTHESIS"},
  {"synthesis validation", "event research: analyze researched event evidence: normalize event analyst synthesis: bad citation", "EVENT_ANALYST_SYNTHESIS_VALIDATION"},
  {"context", "event research: analyze researched event evidence: validate event context: schema invalid", "EVENT_ANALYST_CONTEXT_VALIDATION"},
  {"sources", "event research: index cited event sources: unknown document", "EVENT_SOURCE_VALIDATION"},
  {"false-positive", "market research: run event evidence research loop: invalid", "UNKNOWN"},
  {"credentials init", "load live research credentials: encrypted store corrupted, token=SECRET123", "RESEARCH_CREDENTIAL_LOAD"},
  {"service init", "initialize live research service: invalid provider config token=SECRET123", "RESEARCH_SERVICE_INIT"},
  {"unknown remains unknown", "provider initialization failed token=SECRET123", "UNKNOWN"},
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
