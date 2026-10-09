package research

import (
	"Aegis/internal/agent/event"
	capevent "Aegis/internal/capability/event"
	"Aegis/internal/research/evidenceledger"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func syntheticDocumentBundleInput(t *testing.T) ArchivedResearchInput {
	t.Helper()
	in := syntheticArchiveInput(t, true)
	quote := "fictional archived disclosure text"
	changeArchivedResult(t, &in, func(result *FullPortfolioResult) {
		e := result.ResearchArtifact.Event
		e.Status = event.StatusNormal
		e.Confidence = 0.8
		e.Events = []event.Event{{
			ID:"event-1",Type:event.EventDisclosure,
			Headline:"fictional disclosure",FactSummary:quote,
			Facts:[]event.Fact{{ID:"fact-1",Type:event.EventDisclosure,Headline:"fictional disclosure",Summary:quote,
				Groundings:[]event.EvidenceGrounding{{DocumentID:"doc-1",Quote:quote,StartByte:0,EndByte:len(quote)}},
				EvidenceDocumentIDs:[]string{"doc-1"},Confidence:0.8}},
			EvidenceDocumentIDs:[]string{"doc-1"},
			SourceCount:1,HasPrimarySource:true,
			EvidenceSupport:capevent.EvidenceSupport{DocumentCount:1,IndependentEvidenceCount:1,IndependentSourceCount:1,PrimarySourcePresent:true,SupportClass:capevent.EvidenceSupportPrimaryOnly},
			FactConfidence:0.8,
			Materiality:event.Materiality{Level:event.MaterialityMedium,Direction:event.DirectionUncertain,Horizon:event.HorizonShortTerm,Rationale:"fictional test fixture",Confidence:0.7},
		}}
		e.Evidence=[]string{"doc-1"}
		projected,err:=evidenceledger.Build(result.ResearchArtifact,result.PortfolioReview.Debate,result.PortfolioReview.ResearchPlan)
		if err!=nil{t.Fatalf("invalid fixture ledger: %v",err)}
		result.PortfolioReview.EvidenceLedger=projected
	})
	docBytes:=[]byte(quote)
	docSHA:=sha256.Sum256(docBytes)
	bundle:=ArchivedDocumentBundle{Documents:[]ArchivedDocument{{
		DocumentID:"doc-1",Content:docBytes,SHA256:hex.EncodeToString(docSHA[:]),
		EvidenceAsOf:in.Source.EvidenceAsOf,CapturedAt:in.Source.CapturedAt,
	}}}
	setDocumentBundle(t,&in,bundle)
	return in
}

func setDocumentBundle(t *testing.T,in *ArchivedResearchInput,bundle ArchivedDocumentBundle){
	t.Helper()
	raw,err:=json.Marshal(bundle);if err!=nil{t.Fatal(err)}
	in.EvidenceBytes=raw
	h:=sha256.Sum256(raw)
	in.Source.EvidenceSHA256=hex.EncodeToString(h[:])
}

func TestCaptureArchivedResearchDocumentBundleBindsRealCitedIDs(t *testing.T){
	in:=syntheticDocumentBundleInput(t)
	got,err:=CaptureArchivedResearchDocumentBundle(in)
	if err!=nil{t.Fatal(err)}
	if got.Candidate==nil||got.Candidate.SampleID!=in.SampleID||
		got.Candidate.ResearchSHA256!=in.ResearchSHA256||
		len(got.Documents)!=1||got.Documents[0].DocumentID!="doc-1"||
		got.Documents[0].SHA256==""||got.ReferencedDocumentCount!=1 {
		t.Fatalf("lost provenance or coverage: %+v",got)
	}
	raw,_:=json.Marshal(got)
	if strings.Contains(string(raw),"gold")||strings.Contains(string(raw),"annotations")||
		strings.Contains(string(raw),"fictional archived disclosure text"){
		t.Fatalf("audit leaked evidence contents or invented labels: %s",raw)
	}
	// Legacy capture with opaque bytes remains valid, strict mode is additive.
	if _,err:=CaptureArchivedResearch(syntheticArchiveInput(t,true));err!=nil{t.Fatalf("legacy capture regressed: %v",err)}
}

func TestDocumentBundleRejectsMissingOrMismatchedEvidence(t *testing.T){
	tests:=[]struct{name string; mutate func(*testing.T,*ArchivedResearchInput); want string}{
		{"missing cited document",func(t *testing.T,in *ArchivedResearchInput){
			doc:=ArchivedDocument{DocumentID:"other",Content:[]byte("other"),EvidenceAsOf:in.Source.EvidenceAsOf,CapturedAt:in.Source.CapturedAt}
			h:=sha256.Sum256(doc.Content);doc.SHA256=hex.EncodeToString(h[:])
			setDocumentBundle(t,in,ArchivedDocumentBundle{Documents:[]ArchivedDocument{doc}})
		},"doc-1"},
		{"duplicate document id",func(t *testing.T,in *ArchivedResearchInput){
			var b ArchivedDocumentBundle;_ = json.Unmarshal(in.EvidenceBytes,&b)
			b.Documents=append(b.Documents,b.Documents[0]);setDocumentBundle(t,in,b)
		},"duplicate"},
		{"noncanonical document id",func(t *testing.T,in *ArchivedResearchInput){
			var b ArchivedDocumentBundle;_ = json.Unmarshal(in.EvidenceBytes,&b)
			b.Documents[0].DocumentID=" doc-1 ";setDocumentBundle(t,in,b)
		},"canonical"},
		{"document hash mismatch",func(t *testing.T,in *ArchivedResearchInput){
			var b ArchivedDocumentBundle;_ = json.Unmarshal(in.EvidenceBytes,&b)
			b.Documents[0].SHA256=strings.Repeat("0",64);setDocumentBundle(t,in,b)
		},"sha256"},
		{"empty document",func(t *testing.T,in *ArchivedResearchInput){
			var b ArchivedDocumentBundle;_ = json.Unmarshal(in.EvidenceBytes,&b)
			b.Documents[0].Content=nil;setDocumentBundle(t,in,b)
		},"content"},
		{"captured after bundle capture",func(t *testing.T,in *ArchivedResearchInput){
			var b ArchivedDocumentBundle;_ = json.Unmarshal(in.EvidenceBytes,&b)
			b.Documents[0].CapturedAt="2026-09-23T16:00:00+08:00";setDocumentBundle(t,in,b)
		},"captured_at"},
		{"evidence dated after document capture",func(t *testing.T,in *ArchivedResearchInput){
			var b ArchivedDocumentBundle;_ = json.Unmarshal(in.EvidenceBytes,&b)
			b.Documents[0].EvidenceAsOf="2026-09-23T15:00:00+08:00";setDocumentBundle(t,in,b)
		},"chronology"},
		{"unknown top-level field",func(t *testing.T,in *ArchivedResearchInput){
			bad:=append([]byte(nil),in.EvidenceBytes[:len(in.EvidenceBytes)-1]...)
			bad=append(bad,[]byte(`,"invented":true}`)...)
			in.EvidenceBytes=bad;h:=sha256.Sum256(bad);in.Source.EvidenceSHA256=hex.EncodeToString(h[:])
		},"unknown field"},
		{"extra JSON object",func(t *testing.T,in *ArchivedResearchInput){
			raw:=append(append([]byte(nil),in.EvidenceBytes...),[]byte("{}")...)
			in.EvidenceBytes=raw;h:=sha256.Sum256(raw);in.Source.EvidenceSHA256=hex.EncodeToString(h[:])
		},"multiple"},
	}
	for _,tc:=range tests{t.Run(tc.name,func(t *testing.T){
		in:=syntheticDocumentBundleInput(t)
		tc.mutate(t,&in)
		got,err:=CaptureArchivedResearchDocumentBundle(in)
		if got!=nil||err==nil||!strings.Contains(strings.ToLower(err.Error()),strings.ToLower(tc.want)){
			t.Fatalf("invalid bundle accepted: got=%+v err=%v want=%q",got,err,tc.want)
		}
	})}
}

func TestDocumentBundleRejectsNoCitedEventDocuments(t *testing.T){
	in:=syntheticArchiveInput(t,true)
	doc:=ArchivedDocument{DocumentID:"doc-1",Content:[]byte("fictional"),EvidenceAsOf:in.Source.EvidenceAsOf,CapturedAt:in.Source.CapturedAt}
	h:=sha256.Sum256(doc.Content);doc.SHA256=hex.EncodeToString(h[:])
	setDocumentBundle(t,&in,ArchivedDocumentBundle{Documents:[]ArchivedDocument{doc}})
	got,err:=CaptureArchivedResearchDocumentBundle(in)
	if got!=nil||err==nil||!strings.Contains(err.Error(),"no cited Event documents"){
		t.Fatalf("strict capture accepted no Event citations: %+v %v",got,err)
	}
}
