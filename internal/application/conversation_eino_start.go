package application

import (
 "context"
 "errors"
 "fmt"
 "regexp"
 "strings"
 "time"

 "github.com/cloudwego/eino/components/tool"
 "github.com/cloudwego/eino/components/tool/utils"
)

var conversationSymbolPattern=regexp.MustCompile(`^(?:(?:600|601|603|605|688)[0-9]{3}\.SH|(?:000|001|002|003|300|301|302)[0-9]{3}\.SZ)$`)

// ConversationStartPolicy is supplied by the trusted API/session caller,
// never by an LLM tool argument. Only decision mode is enabled here: portfolio
// snapshots must not be guessed or supplied by a model.
type ConversationStartPolicy struct {
 Cutoff time.Time
 Lookback time.Duration
 // AllowedSymbol optionally pins authorization to one user-confirmed ticker.
 AllowedSymbol string
}

type ConversationStartArgs struct {
 Symbol string `json:"symbol" jsonschema:"required,description=Six digit A-share symbol with SH or SZ exchange suffix"`
 MarketQuery string `json:"market_query,omitempty" jsonschema:"description=Optional research focus up to 1024 bytes"`
}

// EinoStartResearchTool gives a trusted, authorized conversation a constrained
// way to submit an ordinary ResearchRun. It does not execute any extra graph.
func (t *ConversationResearchTools) EinoStartResearchTool(policy ConversationStartPolicy) (tool.InvokableTool,error) {
 if t==nil||t.runs==nil{return nil,errors.New("conversation tools unavailable")}
 if policy.Cutoff.IsZero()||policy.Lookback<=0{return nil,errors.New("conversation start policy requires cutoff and lookback")}
 if policy.AllowedSymbol!="" && !conversationSymbolPattern.MatchString(policy.AllowedSymbol) {return nil,errors.New("invalid authorized research symbol")}
 return utils.InferTool("StartResearch","Start a new Aegis decision research run on the requested A-share symbol; this may call paid research providers. Returns a Run ID, not a research conclusion.",func(ctx context.Context,args ConversationStartArgs)(ConversationRunSummary,error){
  symbol:=strings.ToUpper(strings.TrimSpace(args.Symbol))
  if !conversationSymbolPattern.MatchString(symbol){return ConversationRunSummary{},errors.New("invalid A-share symbol")}
  if policy.AllowedSymbol!="" && symbol!=policy.AllowedSymbol{return ConversationRunSummary{},errors.New("research symbol not authorized for this request")}
  if len(args.MarketQuery)>1024{return ConversationRunSummary{},errors.New("market query too long")}
  input,err:=NewLiveResearchInput(symbol,args.MarketQuery,policy.Cutoff,policy.Lookback)
  if err!=nil{return ConversationRunSummary{},fmt.Errorf("build live research: %w",err)}
  return t.StartResearch(ctx,ResearchRequest{Mode:ModeDecision,Input:input})
 })
}
