package research

import (
	eventagent "Aegis/internal/agent/event"
	"Aegis/internal/agent/technical"
	"Aegis/internal/agent/trading"
	eventcap "Aegis/internal/capability/event"
	technicalcap "Aegis/internal/capability/technical"
	"Aegis/internal/config"
	"Aegis/internal/decision"
	"Aegis/internal/llm"
	"Aegis/internal/research/planning"
	"Aegis/internal/research/synthesis"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/tool"
)

func TestWorkflow_Run_FullStatelessHostedIntegration(t *testing.T) {
	if os.Getenv("AEGIS_FULL_STATELESS_E2E")!="1" { t.Skip("set AEGIS_FULL_STATELESS_E2E=1 to run full stateless hosted E2E") }
	for _,name:=range []string{"DEEPSEEK_API_KEY","TUSHARE_TOKEN","TAVILY_API_KEY"} {
		if strings.TrimSpace(os.Getenv(name))=="" { t.Fatalf("%s is required",name) }
	}
	ctx,cancel:=context.WithTimeout(context.Background(),15*time.Minute);defer cancel()
	modelName:=envOrDefault("DEEPSEEK_MODEL","deepseek-chat")
	strongName:=envOrDefault("DEEPSEEK_STRONG_MODEL",modelName)
	cfg:=config.Config{DefaultModel:"default",Models:map[string]config.ModelConfig{
		"default":{Provider:config.ProviderDeepSeek,Model:modelName,APIKeyEnv:"DEEPSEEK_API_KEY",BaseURL:os.Getenv("DEEPSEEK_BASE_URL")},
		"strong":{Provider:config.ProviderDeepSeek,Model:strongName,APIKeyEnv:"DEEPSEEK_API_KEY",BaseURL:os.Getenv("DEEPSEEK_BASE_URL")},
	},Agents:map[string]config.AgentConfig{"trading":{Model:"strong"}}}
	models:=llm.NewManager(cfg.Models)
	defaultModel,err:=models.Get(ctx,"default");if err!=nil{t.Fatal(err)}
	strongModel,err:=models.Get(ctx,"strong");if err!=nil{t.Fatal(err)}
	planningModel,err:=models.GetToolCalling(ctx,"default");if err!=nil{t.Fatal(err)}

	researcher,err:=technicalcap.NewHTTPResearcher(os.Getenv("TUSHARE_TOKEN"));if err!=nil{t.Fatal(err)}
	researchTool,err:=technicalcap.NewTool(researcher);if err!=nil{t.Fatal(err)}
	technicalAgent,err:=technical.New(ctx,technical.Config{Model:defaultModel,Tools:[]tool.BaseTool{researchTool},MaxIterations:30});if err!=nil{t.Fatal(err)}
	tradingAgent,err:=trading.New(ctx,trading.Config{Model:strongModel,MaxIterations:30});if err!=nil{t.Fatal(err)}

	store,err:=eventcap.NewFileEvidenceSnapshotStore(t.TempDir());if err!=nil{t.Fatal(err)}
	resolver:=&eventcap.CninfoEvidenceTargetResolver{}
	selector:=&eventcap.HybridEvidenceSelector{TargetResolver:resolver,SemanticMatcher:&eventcap.AgenticSemanticEvidenceMatcher{Model:defaultModel}}
	officialSnapshots:=&eventcap.SnapshotRetriever{Store:store,Name:"e2e_snapshot_official",SourceTypes:[]eventcap.SourceType{eventcap.SourceOfficial}}
	breakingSnapshots:=&eventcap.SnapshotRetriever{Store:store,Name:"e2e_snapshot_breaking",SourceTypes:[]eventcap.SourceType{eventcap.SourceIndustryMedia,eventcap.SourceMainstreamMedia,eventcap.SourceOther}}
	analysis,err:=eventagent.NewAgent(ctx,eventagent.Config{
		Planning:planning.PlanExecuteConfig{PlannerModel:planningModel,ReplannerModel:planningModel,Executor:planning.ResearchRoundExecutorConfig{Compiler:planning.Compiler{Channels:planning.ChannelBindings{OfficialDisclosureProviders:[]eventcap.Retriever{officialSnapshots},BreakingNewsProviders:[]eventcap.Retriever{breakingSnapshots},Selector:selector}},Runner:eventcap.AcquisitionPlanRunner{}},MaxIterations:4},
		Analyzer:eventagent.Analyzer{
			Extractor:&eventagent.AgenticEventFactExtractor{Model:defaultModel},
			Clusterer:&eventagent.AgenticEventClusterer{Model:defaultModel},
			Contradiction:&eventagent.AgenticEventContradictionDetector{Model:defaultModel},
			Evolution:&eventagent.AgenticEventFactEvolutionDetector{Model:defaultModel},
			Impact:&eventagent.AgenticEventImpactMechanismAnalyzer{Model:defaultModel},
			Materiality:&eventagent.AgenticEventMaterialityAnalyzer{Model:defaultModel},
			Synthesis:&eventagent.AgenticEventSynthesisAnalyzer{Model:defaultModel},
		},
	});if err!=nil{t.Fatal(err)}
	liveEvent,err:=eventagent.NewLiveAgent(eventagent.LiveAgentConfig{
		Analysis:analysis,CaptureRunner:eventcap.AcquisitionPlanRunner{SnapshotStore:store},
		OfficialRetrievers:[]eventcap.Retriever{&eventcap.CninfoAnnouncementRetriever{}},
		BreakingRetrievers:[]eventcap.Retriever{&eventcap.ClsNewsRetriever{MaxDays:9},&eventcap.TavilyWebSearchRetriever{APIKey:os.Getenv("TAVILY_API_KEY"),TargetResolver:resolver}},
	});if err!=nil{t.Fatal(err)}

	workflow,err:=New(ctx,Config{
		MarketAgent:&hostedMarketAgent{result:hostedMarketContext()},
		TechnicalAgent:technicalAgent,
		EventAgent:liveEvent,
		Synthesizer:&synthesis.AgenticSynthesizer{Model:defaultModel},
		TradingAgent:tradingAgent,
	});if err!=nil{t.Fatal(err)}
	cutoff:=time.Now().UTC()
	result,err:=workflow.Run(ctx,Input{
		Symbol:"600519.SH",RequestedAsOf:cutoff,
		EventInput:eventagent.Input{Task:eventcap.ResearchTask{
			ID:"full-stateless-e2e",Symbol:"600519.SH",Objective:"研究近期公告、新闻与重要事件并形成可追溯事件上下文",InformationNeed:eventcap.NeedEventEvidence,AsOf:cutoff,
			TimeRange:eventcap.TimeRange{Start:cutoff.Add(-7*24*time.Hour),End:cutoff},
			Requirements:eventcap.ResearchRequirements{PITSafe:true,PreferredSourceTypes:[]eventcap.SourceType{eventcap.SourceOfficial,eventcap.SourceIndustryMedia,eventcap.SourceMainstreamMedia},MinDocuments:1,MinIndependentSources:1},
		},Query:"研究请求时点之前的近期公司公告、新闻与重要事件；结合官方公告、CLS 与 Tavily Web evidence。"},
	});if err!=nil{t.Fatalf("run full stateless research workflow: %v",err)}
	if err:=decision.Validate(result);err!=nil{t.Fatalf("validate final decision: %v",err)}
	data,err:=json.MarshalIndent(result,"","  ");if err!=nil{t.Fatal(err)}
	t.Logf("full stateless final decision:\n%s",data)
}
