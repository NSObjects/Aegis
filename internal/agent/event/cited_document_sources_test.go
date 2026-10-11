package event

import (
	"strings"
	"testing"
	"time"

	capevent "Aegis/internal/capability/event"
)

func TestCitedSourceMetadataUsesOnlySelectedCanonicalDocumentsAndNoSecrets(t *testing.T) {
	asOf := time.Date(2026,10,11,10,0,0,0,time.UTC)
	ctx := &Context{Status:StatusNormal,Events:[]Event{{EvidenceDocumentIDs:[]string{"doc-b","doc-a"}}}}
	docs:=[]capevent.CanonicalDocument{
		{ID:"doc-b",VersionID:"version-b",ContentHash:strings.Repeat("b",64),
			Source:capevent.DocumentSource{Publisher:"media",URL:"https://news.example/secure?token=SECRET_DO_NOT_STORE"},
			AvailableAt:&asOf,PublishedAt:&asOf},
		{ID:"doc-a",VersionID:"version-a",ContentHash:strings.Repeat("a",64),
			Source:capevent.DocumentSource{Publisher:"exchange",URL:"https://exchange.example/notice",IsPrimary:true},
			AvailableAt:&asOf,PublishedAt:&asOf},
		{ID:"uncited",VersionID:"version-c",ContentHash:strings.Repeat("c",64),
			Source:capevent.DocumentSource{URL:"https://not-used.example/"},AvailableAt:&asOf},
	}
	got,err:=indexCitedDocumentSources(ctx,docs)
	if err!=nil {t.Fatal(err)}
	if len(got)!=2 || got[0].DocumentID!="doc-a" || got[1].DocumentID!="doc-b" {
		t.Fatalf("not exactly referenced canonical documents: %+v",got)
	}
	if got[0].URL!="https://exchange.example/notice" || got[1].URL!="" ||
		got[0].VersionID!="version-a" || !got[0].IsPrimary ||
		got[1].Publisher!="media" {
		t.Fatalf("unsafe or missing metadata %+v",got)
	}
	ctx.CitedDocumentSources=got
	if err:=validateCitedDocumentSources(ctx);err!=nil{t.Fatal(err)}
	duplicate:=append(append([]CitedDocumentSource{},got...),got[1])
	ctx.CitedDocumentSources=duplicate
	if err:=validateCitedDocumentSources(ctx);err==nil{t.Fatal("accepted duplicated cited sources")}
	ctx.CitedDocumentSources=got
	ctx.CitedDocumentSources[0].URL="https://exchange.example/notice?key=SECRET"
	if err:=validateCitedDocumentSources(ctx);err==nil{t.Fatal("accepted URL containing credentials")}
}

func TestCitedSourceRejectsMissingOrAmbiguousCanonicalDocument(t *testing.T) {
	t0:=time.Date(2026,10,11,10,0,0,0,time.UTC)
	ctx:=&Context{Status:StatusNormal,Events:[]Event{{EvidenceDocumentIDs:[]string{"doc-a"}}}}
	d:=capevent.CanonicalDocument{ID:"doc-a",VersionID:"v1",ContentHash:strings.Repeat("a",64),AvailableAt:&t0}
	if _,err:=indexCitedDocumentSources(ctx,nil);err==nil{t.Fatal("referenced document missing")}
	if _,err:=indexCitedDocumentSources(ctx,[]capevent.CanonicalDocument{d,d});err==nil{t.Fatal("ambiguous canonical IDs")}
	d.AvailableAt=nil
	if _,err:=indexCitedDocumentSources(ctx,[]capevent.CanonicalDocument{d});err==nil{t.Fatal("missing PIT availability accepted")}
	if safeEvidenceURL("https://user:pw@host.example/")!="" || safeEvidenceURL("http://host.example/")!="" ||
		safeEvidenceURL("https://host.example/?access_token=hidden")!="" {
		t.Fatal("unsafe source URL was not redacted")
	}
}
