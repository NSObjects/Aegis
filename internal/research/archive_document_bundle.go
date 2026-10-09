package research

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// ArchivedDocumentBundle is an opt-in, strictly decoded content-addressed
// evidence format. EvidenceBytes in the legacy archive API remains opaque.
// Content uses Go JSON's base64-encoded []byte representation.
type ArchivedDocumentBundle struct {
	Documents []ArchivedDocument `json:"documents"`
}

type ArchivedDocument struct {
	DocumentID string `json:"document_id"`
	Content []byte `json:"content"`
	SHA256 string `json:"sha256"`
	EvidenceAsOf string `json:"evidence_as_of"`
	CapturedAt string `json:"captured_at"`
}

// ArchivedDocumentAudit exposes only IDs and byte digests, not the content,
// a semantic-claim judgment, independent source authentication or Gold labels.
type ArchivedDocumentAudit struct {
	DocumentID string `json:"document_id"`
	SHA256 string `json:"sha256"`
}

type ArchivedDocumentCapture struct {
	Candidate *ResearchSampleCandidate `json:"candidate"`
	Documents []ArchivedDocumentAudit `json:"documents"`
	ReferencedDocumentCount int `json:"referenced_document_count"`
}

// CaptureArchivedResearchDocumentBundle requires the legacy validated research
// result plus an explicit, per-document content-addressed bundle. This check
// establishes citation ID COVERAGE only, not claim-to-document semantic truth.
func CaptureArchivedResearchDocumentBundle(input ArchivedResearchInput) (*ArchivedDocumentCapture, error) {
	candidate, err := CaptureArchivedResearch(input)
	if err != nil { return nil, err }
	var bundle ArchivedDocumentBundle
	decoder := json.NewDecoder(bytes.NewReader(input.EvidenceBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&bundle); err != nil {
		return nil, fmt.Errorf("decode archived document bundle: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil { return nil, errors.New("archived document bundle contains multiple JSON values") }
		return nil, fmt.Errorf("archived document bundle trailing JSON: %w", err)
	}
	if len(bundle.Documents) == 0 { return nil, errors.New("archived document bundle requires documents") }
	sourceEvidenceAt, _ := time.Parse(time.RFC3339, input.Source.EvidenceAsOf)
	sourceCaptureAt, _ := time.Parse(time.RFC3339, input.Source.CapturedAt)
	requestedAt, _ := time.Parse(time.RFC3339, input.Source.RequestedAsOf)
	documents := make(map[string]ArchivedDocumentAudit, len(bundle.Documents))
	for index, doc := range bundle.Documents {
		if strings.TrimSpace(doc.DocumentID)=="" || doc.DocumentID!=strings.TrimSpace(doc.DocumentID) {
			return nil, fmt.Errorf("document %d id must be canonical",index)
		}
		if _, duplicate:=documents[doc.DocumentID];duplicate {
			return nil, fmt.Errorf("duplicate document id %q",doc.DocumentID)
		}
		if len(doc.Content)==0 { return nil, fmt.Errorf("document %q content is required",doc.DocumentID) }
		sum:=sha256.Sum256(doc.Content)
		digest:=hex.EncodeToString(sum[:])
		if doc.SHA256!=digest {return nil,fmt.Errorf("document %q sha256 mismatch",doc.DocumentID)}
		evidenceAt,evidenceErr:=time.Parse(time.RFC3339,doc.EvidenceAsOf)
		capturedAt,captureErr:=time.Parse(time.RFC3339,doc.CapturedAt)
		if evidenceErr!=nil || captureErr!=nil {
			return nil,fmt.Errorf("document %q requires RFC3339 evidence_as_of and captured_at",doc.DocumentID)
		}
		if evidenceAt.After(capturedAt) {return nil,fmt.Errorf("document %q chronology violates evidence_as_of <= captured_at",doc.DocumentID)}
		if capturedAt.After(sourceCaptureAt) || capturedAt.After(requestedAt) {
			return nil,fmt.Errorf("document %q captured_at exceeds archived source cutoff",doc.DocumentID)
		}
		if evidenceAt.After(sourceEvidenceAt) {
			return nil,fmt.Errorf("document %q evidence_as_of exceeds bundle evidence observation cutoff",doc.DocumentID)
		}
		documents[doc.DocumentID]=ArchivedDocumentAudit{DocumentID:doc.DocumentID,SHA256:digest}
	}
	// The existing capture performed strict full-result decoding plus Ledger
	// reconstruction; this second decode only reads its already-hashed bytes.
	var saved FullPortfolioResult
	if err:=json.Unmarshal(input.ResearchJSON,&saved);err!=nil{
		return nil,fmt.Errorf("decode validated archived research: %w",err)
	}
	if saved.ResearchArtifact==nil || saved.ResearchArtifact.Event==nil {
		return nil,errors.New("archived EventContext is missing")
	}
	referenced:=make(map[string]struct{})
	for _,event:=range saved.ResearchArtifact.Event.Events {
		for _,id:=range event.EvidenceDocumentIDs { referenced[id]=struct{}{} }
		for _,fact:=range event.Facts {
			for _,id:=range fact.EvidenceDocumentIDs { referenced[id]=struct{}{} }
			for _,grounding:=range fact.Groundings { referenced[grounding.DocumentID]=struct{}{} }
		}
	}
	if len(referenced)==0 {
		return nil,errors.New("archived research has no cited Event documents")
	}
	ids:=make([]string,0,len(referenced))
	for id:=range referenced {
		if _,exists:=documents[id];!exists {return nil,fmt.Errorf("cited Event document %q missing from archive bundle",id)}
		ids=append(ids,id)
	}
	sort.Strings(ids)
	result:=&ArchivedDocumentCapture{Candidate:candidate,ReferencedDocumentCount:len(ids)}
	for _,id:=range ids {result.Documents=append(result.Documents,documents[id])}
	return result,nil
}
