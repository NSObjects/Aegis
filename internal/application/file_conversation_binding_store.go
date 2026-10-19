package application

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "sync"
)

// FileConversationBindingStore stores minimal canonical Run references.
// It is a single-process store; do not share it between API replicas.
type FileConversationBindingStore struct {
 dir string
 mu sync.Mutex
 runs interface{Get(context.Context,string)(RunRecord,error)}
}

func NewFileConversationBindingStore(dir string,runs interface{Get(context.Context,string)(RunRecord,error)})(*FileConversationBindingStore,error){
 if strings.TrimSpace(dir)==""||runs==nil{return nil,errors.New("session store requires directory and run lookup")}
 if err:=os.MkdirAll(dir,0700);err!=nil{return nil,err}
 info,err:=os.Stat(dir);if err!=nil{return nil,err}
 if !info.IsDir()||info.Mode().Perm()&0077!=0{return nil,errors.New("session directory must be private")}
 return &FileConversationBindingStore{dir:dir,runs:runs},nil
}
func (s *FileConversationBindingStore) path(id string)string{
 hash:=sha256.Sum256([]byte(id))
 return filepath.Join(s.dir,hex.EncodeToString(hash[:])+".json")
}
func (s *FileConversationBindingStore) read(id string)(ConversationSessionBinding,error){
 doc,err:=s.readDocument(id)
 if err!=nil{return ConversationSessionBinding{},err}
 return doc.ConversationSessionBinding,nil
}
// conversationBindingDocument remains backward compatible with the original
// single-binding JSON representation and stores a bounded history of Run refs.
type conversationBindingDocument struct {
 ConversationSessionBinding
 History []ConversationSessionBinding `json:"history,omitempty"`
}
const maxConversationRunHistory = 32

func (s *FileConversationBindingStore) readDocument(id string)(conversationBindingDocument,error) {
 raw,err:=os.ReadFile(s.path(id))
 if errors.Is(err,os.ErrNotExist){return conversationBindingDocument{},ErrConversationSessionUnavailable}
 if err!=nil{return conversationBindingDocument{},err}
 if len(raw)>16384{return conversationBindingDocument{},ErrConversationSessionUnavailable}
 var doc conversationBindingDocument
 if json.Unmarshal(raw,&doc)!=nil||doc.SessionID!=id||doc.OwnerID==""||!validRunID(doc.RunID)||len(doc.History)>maxConversationRunHistory{return conversationBindingDocument{},ErrConversationSessionUnavailable}
 for _,entry:=range doc.History {
  if entry.SessionID!=id||entry.OwnerID!=doc.OwnerID||!validRunID(entry.RunID){return conversationBindingDocument{},ErrConversationSessionUnavailable}
 }
 return doc,nil
}

// ResolveHistory returns server-owned canonical references only, newest first.
// No caller-supplied symbol or Run ID may broaden the session's scope.
func (s *FileConversationBindingStore) ResolveHistory(sessionID,ownerID string)([]ConversationSessionBinding,error) {
 if s==nil||len(sessionID)<8||strings.TrimSpace(ownerID)==""{return nil,ErrConversationSessionUnavailable}
 s.mu.Lock();defer s.mu.Unlock()
 doc,err:=s.readDocument(sessionID)
 if err!=nil{return nil,err}
 if doc.OwnerID!=ownerID{return nil,ErrConversationSessionUnavailable}
 out:=make([]ConversationSessionBinding,0,len(doc.History)+1)
 out=append(out,doc.ConversationSessionBinding)
 out=append(out,doc.History...)
 return out,nil
}

func (s *FileConversationBindingStore) Bind(ctx context.Context,sessionID,ownerID,runID string)(ConversationSessionBinding,error){
 if s==nil||s.runs==nil||len(sessionID)<8||len(sessionID)>128||strings.TrimSpace(ownerID)==""||!validRunID(runID){return ConversationSessionBinding{},ErrConversationSessionUnavailable}
 rec,err:=s.runs.Get(ctx,runID);if err!=nil{return ConversationSessionBinding{},err}
 item:=ConversationSessionBinding{SessionID:sessionID,OwnerID:ownerID,RunID:rec.ID,Symbol:rec.Symbol,AsOf:rec.RequestedAsOf}
 s.mu.Lock();defer s.mu.Unlock()
 previous,err:=s.readDocument(sessionID)
 if err==nil&&previous.OwnerID!=ownerID{return ConversationSessionBinding{},ErrConversationSessionUnavailable}
 if err!=nil&&!errors.Is(err,ErrConversationSessionUnavailable){return ConversationSessionBinding{},err}
 doc:=conversationBindingDocument{ConversationSessionBinding:item}
 if err==nil {
  seen:=map[string]bool{item.RunID:true}
  for _,entry:=range append([]ConversationSessionBinding{previous.ConversationSessionBinding},previous.History...) {
   if !seen[entry.RunID] {
    doc.History=append(doc.History,entry)
    seen[entry.RunID]=true
   }
   if len(doc.History)>=maxConversationRunHistory {break}
  }
 }
 raw,err:=json.Marshal(doc);if err!=nil{return ConversationSessionBinding{},err}
 tmp,err:=os.CreateTemp(s.dir,".binding-");if err!=nil{return ConversationSessionBinding{},err}
 defer os.Remove(tmp.Name())
 if err=tmp.Chmod(0600);err!=nil{tmp.Close();return ConversationSessionBinding{},err}
 if _,err=tmp.Write(raw);err!=nil{tmp.Close();return ConversationSessionBinding{},err}
 if err=tmp.Sync();err!=nil{tmp.Close();return ConversationSessionBinding{},err}
 if err=tmp.Close();err!=nil{return ConversationSessionBinding{},err}
 if err=os.Rename(tmp.Name(),s.path(sessionID));err!=nil{return ConversationSessionBinding{},err}
 return item,nil
}
func (s *FileConversationBindingStore) Resolve(sessionID,ownerID string)(ConversationSessionBinding,error){
 if s==nil||len(sessionID)<8||strings.TrimSpace(ownerID)==""{return ConversationSessionBinding{},ErrConversationSessionUnavailable}
 s.mu.Lock();defer s.mu.Unlock()
 item,err:=s.read(sessionID);if err!=nil{return ConversationSessionBinding{},err}
 if item.OwnerID!=ownerID{return ConversationSessionBinding{},ErrConversationSessionUnavailable}
 return item,nil
}
