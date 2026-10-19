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
 raw,err:=os.ReadFile(s.path(id));if errors.Is(err,os.ErrNotExist){return ConversationSessionBinding{},ErrConversationSessionUnavailable};if err!=nil{return ConversationSessionBinding{},err}
 if len(raw)>4096{return ConversationSessionBinding{},ErrConversationSessionUnavailable}
 var item ConversationSessionBinding
 if json.Unmarshal(raw,&item)!=nil||item.SessionID!=id||item.OwnerID==""||!validRunID(item.RunID){return ConversationSessionBinding{},ErrConversationSessionUnavailable}
 return item,nil
}
func (s *FileConversationBindingStore) Bind(ctx context.Context,sessionID,ownerID,runID string)(ConversationSessionBinding,error){
 if s==nil||s.runs==nil||len(sessionID)<8||len(sessionID)>128||strings.TrimSpace(ownerID)==""||!validRunID(runID){return ConversationSessionBinding{},ErrConversationSessionUnavailable}
 rec,err:=s.runs.Get(ctx,runID);if err!=nil{return ConversationSessionBinding{},err}
 item:=ConversationSessionBinding{SessionID:sessionID,OwnerID:ownerID,RunID:rec.ID,Symbol:rec.Symbol,AsOf:rec.RequestedAsOf}
 s.mu.Lock();defer s.mu.Unlock()
 previous,err:=s.read(sessionID)
 if err==nil&&previous.OwnerID!=ownerID{return ConversationSessionBinding{},ErrConversationSessionUnavailable}
 if err!=nil&&!errors.Is(err,ErrConversationSessionUnavailable){return ConversationSessionBinding{},err}
 if err==nil&&previous.OwnerID==ownerID{} // same owner may update reference
 raw,err:=json.Marshal(item);if err!=nil{return ConversationSessionBinding{},err}
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
