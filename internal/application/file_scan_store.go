package application

import (
 "crypto/rand"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "sort"
 "strings"
 "sync"
 "time"

 "Aegis/internal/capability/technical"
 "Aegis/internal/strategy/pullback"
)

type ScanState string
const (
 ScanPending ScanState = "PENDING"
 ScanRunning ScanState = "RUNNING"
 ScanCompleted ScanState = "COMPLETED"
 ScanFailed ScanState = "FAILED"
)

// ScanRecord is separate from the LLM ResearchRun contract. It never
// represents an executable order or a model-produced research conclusion.
type ScanRecord struct {
 ID string `json:"scan_id"`
 AsOf string `json:"as_of"`
 StrategyVersion string `json:"strategy_version"`
 State ScanState `json:"state"`
 UniverseSource string `json:"universe_source"`
 InputSnapshotSHA256 string `json:"input_snapshot_sha256"`
 Symbols []string `json:"symbols"`
 Results []pullback.Result `json:"results,omitempty"`
 // Snapshots reference each symbol's persisted input observation file by
 // digest; present only on completed scans with actually observed inputs.
 Snapshots []ScanSnapshotRef `json:"snapshots,omitempty"`
 FailureCode string `json:"failure_code,omitempty"`
 CreatedAt time.Time `json:"created_at"`
 UpdatedAt time.Time `json:"updated_at"`
}

// ScanSnapshotRef anchors one symbol's persisted input observation. SHA256 is
// computed over the canonical observation file so any verdict can be
// re-verified offline against the exact bars and PIT factors it consumed.
type ScanSnapshotRef struct {
 Symbol string `json:"symbol"`
 SHA256 string `json:"sha256"`
 BarsCount int `json:"bars_count"`
 EffectiveAsOf string `json:"effective_as_of"`
}

type FileScanStore struct { dir string; mu sync.Mutex }
func NewFileScanStore(dir string)(*FileScanStore,error){
 if strings.TrimSpace(dir)==""{return nil,errors.New("scan directory required")}
 if err:=os.MkdirAll(dir,0700);err!=nil{return nil,err}
 info,err:=os.Stat(dir);if err!=nil{return nil,err}
 if !info.IsDir()||info.Mode().Perm()&0077!=0{return nil,errors.New("scan directory must be private")}
 return &FileScanStore{dir:dir},nil
}
func NewScanID()(string,error){
 buf:=make([]byte,16);if _,err:=rand.Read(buf);err!=nil{return "",err}
 return hex.EncodeToString(buf),nil
}
func validScanID(s string)bool{
 if len(s)!=32{return false};_,err:=hex.DecodeString(s);return err==nil&&s==strings.ToLower(s)
}
// ScanInputSnapshotSHA256 fingerprints the canonical scan inputs. It records
// the PIT cutoff, strategy version, universe provenance and ordered symbols;
// it does not claim to fingerprint the provider's underlying market bars.
func ScanInputSnapshotSHA256(r ScanRecord) string {
 canonical:=r.AsOf+"\n"+r.StrategyVersion+"\n"+r.UniverseSource+"\n"+strings.Join(r.Symbols,"\n")
 sum:=sha256.Sum256([]byte(canonical))
 return hex.EncodeToString(sum[:])
}

func validateScanRecord(r ScanRecord)error{
 if !validScanID(r.ID)||r.AsOf==""||r.StrategyVersion!=pullback.StrategyVersion||r.CreatedAt.IsZero()||r.UpdatedAt.IsZero()||r.UpdatedAt.Before(r.CreatedAt){return errors.New("invalid scan metadata")}
 date,err:=time.Parse("20060102",r.AsOf);if err!=nil||date.Format("20060102")!=r.AsOf{return errors.New("invalid PIT scan cutoff")}
 if len(r.Symbols)==0||len(r.Symbols)>20{return errors.New("invalid bounded scan universe")}
 if r.UniverseSource==""||r.InputSnapshotSHA256==""||r.InputSnapshotSHA256!=ScanInputSnapshotSHA256(r){return errors.New("scan input snapshot mismatch")}
 seen:=map[string]bool{}
 for i,s:=range r.Symbols {if len(s)!=9||(!strings.HasSuffix(s,".SH")&&!strings.HasSuffix(s,".SZ"))||seen[s]||(i>0&&r.Symbols[i-1]>=s){return errors.New("invalid scan symbols")};seen[s]=true}
 switch r.State {
 case ScanPending,ScanRunning:
  if len(r.Results)!=0||len(r.Snapshots)!=0||r.FailureCode!=""{return errors.New("unfinished scan has results or failure")}
 case ScanCompleted:
  if len(r.Results)!=len(r.Symbols)||r.FailureCode!=""{return errors.New("incomplete scan results")}
  for i,v:=range r.Results {
   if v.Symbol!=r.Symbols[i]||v.AsOf!=r.AsOf||v.Version!=r.StrategyVersion{return errors.New("unmatched scan result")}
   if v.Verdict!=pullback.Selected&&v.Verdict!=pullback.Rejected&&v.Verdict!=pullback.InsufficientData{return errors.New("invalid scan verdict")}
  }
  if err:=validateScanSnapshots(r);err!=nil{return err}
 case ScanFailed:
  if r.FailureCode==""||len(r.Results)!=0||len(r.Snapshots)!=0||len(r.FailureCode)>64{return errors.New("invalid failed scan")}
 default:return errors.New("invalid scan state")
 }
 return nil
}

// validateScanSnapshots keeps snapshot references aligned with the universe:
// a strict ordered subset of Symbols, unique, digest-shaped, and pointing at
// a real observed series. Symbols without a valid observation have no file.
func validateScanSnapshots(r ScanRecord)error{
 index:=map[string]int{}
 for i,s:=range r.Symbols{index[s]=i}
 previousPos:=-1
 for _,ref:=range r.Snapshots{
  pos,ok:=index[ref.Symbol];if !ok{return errors.New("snapshot symbol outside scan universe")}
  if pos<=previousPos{return errors.New("snapshots not in universe order")}
  previousPos=pos
  if len(ref.SHA256)!=64{return errors.New("invalid snapshot digest")}
  if _,err:=hex.DecodeString(ref.SHA256);err!=nil||ref.SHA256!=strings.ToLower(ref.SHA256){return errors.New("invalid snapshot digest")}
  if ref.BarsCount<1||ref.EffectiveAsOf==""||ref.EffectiveAsOf>r.AsOf{return errors.New("invalid snapshot observation metadata")}
 }
 return nil
}
func (s *FileScanStore) Save(r ScanRecord)error{
 if s==nil{return errors.New("nil scan store")}
 // Populate the deterministic snapshot for callers using the original record contract.
 // Existing-record immutability checks below prevent changing inputs mid-lifecycle.
 if r.InputSnapshotSHA256=="" {r.InputSnapshotSHA256=ScanInputSnapshotSHA256(r)}
 if err:=validateScanRecord(r);err!=nil{return err}
 s.mu.Lock();defer s.mu.Unlock()
 path:=filepath.Join(s.dir,r.ID+".json")
 if old,err:=os.ReadFile(path);err==nil{
  var prior ScanRecord;if json.Unmarshal(old,&prior)!=nil||validateScanRecord(prior)!=nil{return errors.New("corrupt previous scan record")}
  if prior.State==ScanCompleted||prior.State==ScanFailed{return errors.New("terminal scan immutable")}
  if (prior.State==ScanRunning&&r.State==ScanPending)||prior.ID!=r.ID||prior.AsOf!=r.AsOf||prior.StrategyVersion!=r.StrategyVersion||prior.UniverseSource!=r.UniverseSource||prior.CreatedAt!=r.CreatedAt||len(prior.Symbols)!=len(r.Symbols){return errors.New("invalid scan transition")}
  for i:=range prior.Symbols{if prior.Symbols[i]!=r.Symbols[i]{return errors.New("scan universe changed")}}
 }else if !os.IsNotExist(err){return err}
 raw,err:=json.Marshal(r);if err!=nil{return err}
 tmp,err:=os.CreateTemp(s.dir,".scan-*.tmp");if err!=nil{return err}
 defer os.Remove(tmp.Name())
 if err=tmp.Chmod(0600);err==nil{_,err=tmp.Write(raw)}
 if err==nil{err=tmp.Sync()};closeErr:=tmp.Close();if err!=nil{return err};if closeErr!=nil{return closeErr}
 if err=os.Rename(tmp.Name(),path);err!=nil{return fmt.Errorf("write scan: %w",err)}
 return nil
}
func (s *FileScanStore) Get(id string)(ScanRecord,error){
 if s==nil||!validScanID(id){return ScanRecord{},errors.New("invalid scan lookup")}
 s.mu.Lock();defer s.mu.Unlock()
 raw,err:=os.ReadFile(filepath.Join(s.dir,id+".json"));if err!=nil{return ScanRecord{},err}
 if len(raw)>4<<20{return ScanRecord{},errors.New("scan record too large")}
 var r ScanRecord;if err=json.Unmarshal(raw,&r);err!=nil{return ScanRecord{},err}
 if err=validateScanRecord(r);err!=nil{return ScanRecord{},err}
 return r,nil
}

// ReconcileInterrupted marks unfinished records from a previous process as
// failed. Call at single-process startup, before accepting new scans.
// Corrupt records fail startup rather than being silently skipped.
func (s *FileScanStore) ReconcileInterrupted() (int,error) {
 if s==nil{return 0,errors.New("nil scan store")}
 entries,err:=os.ReadDir(s.dir);if err!=nil{return 0,err}
 ids:=make([]string,0,len(entries))
 for _,entry:=range entries{
  if entry.IsDir()||!strings.HasSuffix(entry.Name(),".json"){continue}
  id:=strings.TrimSuffix(entry.Name(),".json")
  if !validScanID(id){return 0,fmt.Errorf("invalid scan filename %q",entry.Name())}
  ids=append(ids,id)
 }
 sort.Strings(ids)
 changed:=0
 for _,id:=range ids{
  record,err:=s.Get(id);if err!=nil{return changed,fmt.Errorf("read scan %s: %w",id,err)}
  if record.ID!=id{return changed,fmt.Errorf("scan filename/id mismatch %s",id)}
  if record.State!=ScanPending&&record.State!=ScanRunning{continue}
  record.State=ScanFailed
  record.FailureCode="PROCESS_INTERRUPTED"
  record.UpdatedAt=time.Now().UTC()
  if record.UpdatedAt.Before(record.CreatedAt){record.UpdatedAt=record.CreatedAt}
  if err:=s.Save(record);err!=nil{return changed,err}
  changed++
 }
 return changed,nil
}

// List returns bounded scan history newest first. Invalid records fail closed.
func (s *FileScanStore) List(limit int)([]ScanRecord,error){
 if s==nil{return nil,errors.New("nil scan store")}
 if limit<1||limit>100{return nil,errors.New("scan history limit must be 1..100")}
 entries,err:=os.ReadDir(s.dir);if err!=nil{return nil,err}
 records:=make([]ScanRecord,0,len(entries))
 for _,entry:=range entries{
  if entry.IsDir()||!strings.HasSuffix(entry.Name(),".json"){continue}
  id:=strings.TrimSuffix(entry.Name(),".json")
  if !validScanID(id){return nil,errors.New("invalid scan history filename")}
  rec,err:=s.Get(id);if err!=nil{return nil,err}
  if rec.ID!=id{return nil,errors.New("scan history id mismatch")}
  records=append(records,rec)
 }
 sort.Slice(records,func(i,j int)bool{
  if records[i].CreatedAt.Equal(records[j].CreatedAt){return records[i].ID>records[j].ID}
  return records[i].CreatedAt.After(records[j].CreatedAt)
 })
 if len(records)>limit{records=records[:limit]}
 return records,nil
}

// snapshotDir isolates per-scan observation files from record JSON files so
// history listing never traverses snapshot payloads.
func (s *FileScanStore) snapshotDir(scanID string)string{
 return filepath.Join(s.dir,"snapshots",scanID)
}

// SaveSnapshot durably persists one symbol's exact validated observation and
// returns its canonical-JSON digest. Files live under a per-scan directory.
func (s *FileScanStore) SaveSnapshot(scanID string,o technical.HistoricalObservation)(string,error){
 if s==nil{return "",errors.New("nil scan store")}
 if !validScanID(scanID){return "",errors.New("invalid scan ID for snapshot")}
 if len(o.Symbol)!=9||(!strings.HasSuffix(o.Symbol,".SH")&&!strings.HasSuffix(o.Symbol,".SZ")){return "",errors.New("invalid snapshot symbol")}
 if o.PriceBasis!="RAW"||o.EffectiveAsOf==""||len(o.Bars)==0||len(o.Bars)>512{return "",errors.New("invalid snapshot observation")}
 if o.Bars[len(o.Bars)-1].TradeDate!=o.EffectiveAsOf||o.EffectiveAsOf>o.RequestedAsOf{return "",errors.New("snapshot observation identity mismatch")}
 if len(o.PITFactors)>0&&len(o.PITFactors)!=len(o.Bars){return "",errors.New("snapshot PIT factors incomplete")}
 raw,err:=json.Marshal(o);if err!=nil{return "",err}
 s.mu.Lock();defer s.mu.Unlock()
 dir:=s.snapshotDir(scanID)
 if err:=os.MkdirAll(dir,0700);err!=nil{return "",err}
 tmp,err:=os.CreateTemp(dir,".snapshot-*.tmp");if err!=nil{return "",err}
 defer os.Remove(tmp.Name())
 if err=tmp.Chmod(0600);err==nil{_,err=tmp.Write(raw)}
 if err==nil{err=tmp.Sync()};closeErr:=tmp.Close();if err!=nil{return "",err};if closeErr!=nil{return "",closeErr}
 if err=os.Rename(tmp.Name(),filepath.Join(dir,o.Symbol+".json"));err!=nil{return "",fmt.Errorf("write snapshot: %w",err)}
 sum:=sha256.Sum256(raw)
 return hex.EncodeToString(sum[:]),nil
}

// GetSnapshot loads one persisted observation and fails closed when the file
// no longer re-projects to the digest recorded on the scan.
func (s *FileScanStore) GetSnapshot(scanID,symbol,expectedSHA256 string)(technical.HistoricalObservation,error){
 if s==nil||!validScanID(scanID){return technical.HistoricalObservation{},errors.New("invalid scan lookup")}
 if len(symbol)!=9||(!strings.HasSuffix(symbol,".SH")&&!strings.HasSuffix(symbol,".SZ")){return technical.HistoricalObservation{},errors.New("invalid snapshot symbol")}
 s.mu.Lock();defer s.mu.Unlock()
 raw,err:=os.ReadFile(filepath.Join(s.snapshotDir(scanID),symbol+".json"))
 if os.IsNotExist(err){return technical.HistoricalObservation{},os.ErrNotExist}
 if err!=nil{return technical.HistoricalObservation{},err}
 if len(raw)>1<<20{return technical.HistoricalObservation{},errors.New("snapshot file too large")}
 var o technical.HistoricalObservation
 if err:=json.Unmarshal(raw,&o);err!=nil{return technical.HistoricalObservation{},err}
 reprojected,err:=json.Marshal(o);if err!=nil{return technical.HistoricalObservation{},err}
 sum:=sha256.Sum256(reprojected)
 if hex.EncodeToString(sum[:])!=expectedSHA256{return technical.HistoricalObservation{},errors.New("snapshot digest mismatch")}
 if o.Symbol!=symbol{return technical.HistoricalObservation{},errors.New("snapshot symbol mismatch")}
 return o,nil
}
