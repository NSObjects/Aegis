package main

import (
	"Aegis/internal/application"
	eventagent "Aegis/internal/agent/event"
	eventcap "Aegis/internal/capability/event"
	"Aegis/internal/portfolio"
	"Aegis/internal/research"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

func main() {
	var (
		configPath  string
		skillsDir   string
		timeZone    string
		symbol      string
		marketQuery   string
		evidenceStore string
		eventLookback time.Duration
		portfolioReview bool
		portfolioFile string
		portfolioMaxAge time.Duration
		riskLimitsFile string
		tradeScenarioFile string
		withFundamental bool
	)

	flag.StringVar(
		&configPath,
		"config",
		"configs/config.yaml",
		"application config path",
	)
	flag.StringVar(
		&skillsDir,
		"skills",
		"skills",
		"skills directory",
	)
	flag.StringVar(
		&timeZone,
		"timezone",
		"Asia/Shanghai",
		"runtime timezone",
	)
	flag.StringVar(
		&symbol,
		"symbol",
		"600519.SH",
		"research symbol",
	)
	flag.StringVar(
		&marketQuery,
		"market-query",
		"",
		"optional market research query",
	)
	flag.StringVar(
		&evidenceStore,
		"evidence-store",
		".aegis/evidence",
		"event evidence snapshot store",
	)
	flag.DurationVar(
		&eventLookback,
		"event-lookback",
		7*24*time.Hour,
		"live event publication lookback",
	)
	flag.BoolVar(
		&portfolioReview,
		"portfolio-review",
		false,
		"run full Research/Trading/Risk/Portfolio workflow (requires -portfolio-file)",
	)
	flag.StringVar(
		&portfolioFile,
		"portfolio-file",
		"",
		"JSON portfolio context, e.g. {\"state\":\"NOT_PROVIDED\"}",
	)
	flag.DurationVar(
		&portfolioMaxAge,
		"portfolio-max-age",
		0,
		"optional maximum age of observed portfolio snapshot (e.g. 24h); 0 disables this policy",
	)
	flag.StringVar(
		&riskLimitsFile,
		"risk-limits-file",
		"",
		"optional caller-supplied JSON current-risk policy; requires -portfolio-review",
	)
	flag.StringVar(
		&tradeScenarioFile,
		"trade-scenario-file",
		"",
		"optional explicitly supplied CNY trade what-if; requires -portfolio-review and -risk-limits-file",
	)
	flag.BoolVar(&withFundamental,"fundamental",false,"run the actual Fundamental Analyst; requires TUSHARE_TOKEN and TuShare income access")
	flag.Parse()

	if err := runWithFundamental(
		context.Background(),
		configPath,
		skillsDir,
		timeZone,
		symbol,
		marketQuery,
		evidenceStore,
		eventLookback,
		portfolioReview,
		portfolioFile,
		portfolioMaxAge,
		riskLimitsFile,
		tradeScenarioFile,
		withFundamental,
	); err != nil {
		log.Fatal(err)
	}
}

func run(
	ctx context.Context,
	configPath string,
	skillsDir string,
	timeZone string,
	symbol string,
	marketQuery string,
	evidenceStore string,
	eventLookback time.Duration,
	portfolioReview bool,
	portfolioFile string,
	portfolioMaxAge time.Duration,
) error {
	return runWithLimits(ctx, configPath, skillsDir, timeZone, symbol,
		marketQuery, evidenceStore, eventLookback, portfolioReview,
		portfolioFile, portfolioMaxAge, "")
}

// runWithLimits extends the existing CLI path while preserving the original
// run() surface for callers that do not supply a policy.
func runWithLimits(
	ctx context.Context,
	configPath string,
	skillsDir string,
	timeZone string,
	symbol string,
	marketQuery string,
	evidenceStore string,
	eventLookback time.Duration,
	portfolioReview bool,
	portfolioFile string,
	portfolioMaxAge time.Duration,
	riskLimitsFile string,
) error {
	return runWithScenario(ctx, configPath, skillsDir, timeZone, symbol,
		marketQuery, evidenceStore, eventLookback, portfolioReview,
		portfolioFile, portfolioMaxAge, riskLimitsFile, "")
}

// runWithScenario adds a separately declared hypothetical notional transfer
// without altering the existing caller API or auto-sizing a TradeIntent.
func runWithScenario(
	ctx context.Context,
	configPath string,
	skillsDir string,
	timeZone string,
	symbol string,
	marketQuery string,
	evidenceStore string,
	eventLookback time.Duration,
	portfolioReview bool,
	portfolioFile string,
	portfolioMaxAge time.Duration,
	riskLimitsFile string,
	tradeScenarioFile string,
) error {
	return runWithFundamental(ctx,configPath,skillsDir,timeZone,symbol,marketQuery,evidenceStore,eventLookback,portfolioReview,portfolioFile,portfolioMaxAge,riskLimitsFile,tradeScenarioFile,false)
}

func runWithFundamental(
	ctx context.Context,
	configPath string,
	skillsDir string,
	timeZone string,
	symbol string,
	marketQuery string,
	evidenceStore string,
	eventLookback time.Duration,
	portfolioReview bool,
	portfolioFile string,
	portfolioMaxAge time.Duration,
	riskLimitsFile string,
	tradeScenarioFile string,
	withFundamental bool,
) error {
	// Validate account input before initiating any live research or LLM call.
	// NOT_PROVIDED is explicit; no implicit empty portfolios.
	var book *portfolio.Context
	if portfolioReview {
		loaded, err := loadPortfolioContext(portfolioFile)
		if err != nil {
			return err
		}
		book = loaded
	} else if strings.TrimSpace(portfolioFile) != "" {
		return fmt.Errorf("-portfolio-file requires -portfolio-review")
	}
	if !portfolioReview && strings.TrimSpace(riskLimitsFile) != "" {
		return fmt.Errorf("-risk-limits-file requires -portfolio-review")
	}
	var limits *portfolio.Limits
	if strings.TrimSpace(riskLimitsFile) != "" {
		loaded, err := loadRiskLimits(riskLimitsFile)
		if err != nil {
			return err
		}
		limits = loaded
	}
	if strings.TrimSpace(tradeScenarioFile) != "" && !portfolioReview {
		return fmt.Errorf("-trade-scenario-file requires -portfolio-review")
	}
	if strings.TrimSpace(tradeScenarioFile) != "" && limits == nil {
		return fmt.Errorf("-trade-scenario-file requires -risk-limits-file")
	}
	var scenario *portfolio.TradeScenario
	if strings.TrimSpace(tradeScenarioFile) != "" {
		loaded, err := loadTradeScenario(tradeScenarioFile)
		if err != nil {
			return err
		}
		scenario = loaded
	}
	if portfolioMaxAge < 0 {
		return fmt.Errorf("-portfolio-max-age must not be negative")
	}
	if !portfolioReview && portfolioMaxAge != 0 {
		return fmt.Errorf("-portfolio-max-age requires -portfolio-review")
	}

	// Establish the single live research cutoff before constructing any LLM
	// or data source. Snapshot freshness is assessed against this cutoff.
	location, err := time.LoadLocation(timeZone)
	if err != nil {
		return fmt.Errorf("load research timezone %q: %w", timeZone, err)
	}
	eventCutoff := time.Now().In(location)
	workflowInput, err := newLiveResearchInput(
		symbol, marketQuery, eventCutoff, eventLookback,
	)
	if err != nil {
		return err
	}
	if portfolioReview {
		if err := portfolio.ValidateAt(book, workflowInput.RequestedAsOf, portfolioMaxAge); err != nil {
			return fmt.Errorf("invalid portfolio snapshot: %w", err)
		}
		// Fail before loading LLM configuration and external capabilities.
		if _, err := portfolio.EvaluateCurrentLimits(book, limits, workflowInput.RequestedAsOf); err != nil {
			return fmt.Errorf("invalid current risk limits: %w", err)
		}
		if err := portfolio.ValidateTradeScenarioAt(scenario, book, limits,
			workflowInput.RequestedAsOf, workflowInput.Symbol); err != nil {
			return fmt.Errorf("invalid trade scenario: %w", err)
		}
	}

	// Stage progress goes to stderr; stdout stays a single JSON document.
	// Enable after validating caller inputs so invalid invocations stay quiet.
	ctx = research.WithProgress(ctx, os.Stderr)
	fmt.Fprintf(os.Stderr, "[Aegis] stage=Workflow status=START symbol=%s fundamental=%t portfolio_review=%t\n", workflowInput.Symbol, withFundamental, portfolioReview)
	service, err := application.NewLiveResearchService(ctx, application.LiveServiceOptions{
		ConfigPath: configPath,
		SkillsDir: skillsDir,
		TimeZone: timeZone,
		EvidenceStore: evidenceStore,
		EventLookback: eventLookback,
		WithFundamental: withFundamental,
		PortfolioReview: portfolioReview,
		PortfolioMaxAge: portfolioMaxAge,
		Limits: limits,
		Scenario: scenario,
	})
	if err != nil { return err }
	mode := application.ModeDecision
	if portfolioReview { mode = application.ModePortfolio }
	response, err := service.Run(ctx, application.ResearchRequest{
		Mode: mode,
		Input: workflowInput,
		Portfolio: book,
	})
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(response.Output(), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal research result: %w", err)
	}

	fmt.Fprintln(os.Stderr, "[Aegis] stage=Workflow status=DONE")
	fmt.Println(string(data))

	return nil
}


func newLiveResearchInput(
	symbol string,
	marketQuery string,
	cutoff time.Time,
	lookback time.Duration,
) (research.Input, error) {
	symbol = strings.ToUpper(
		strings.TrimSpace(symbol),
	)
	if symbol == "" {
		return research.Input{}, fmt.Errorf(
			"research symbol is required",
		)
	}
	if cutoff.IsZero() {
		return research.Input{}, fmt.Errorf(
			"event publication cutoff is required",
		)
	}
	if lookback <= 0 {
		return research.Input{}, fmt.Errorf(
			"event lookback must be positive",
		)
	}

	return research.Input{
		Symbol:        symbol,
		RequestedAsOf: cutoff,
		MarketQuery:   strings.TrimSpace(marketQuery),
		EventInput: eventagent.Input{
			Task: eventcap.ResearchTask{
				ID: fmt.Sprintf(
					"live-event-%s-%d",
					symbol,
					cutoff.UnixNano(),
				),
				Symbol: symbol,
				Objective:
					"研究该标的近期公告、新闻与重要事件，并形成可追溯的事件上下文。",
				InformationNeed:
					eventcap.NeedEventEvidence,
				AsOf: cutoff,
				TimeRange: eventcap.TimeRange{
					Start: cutoff.Add(-lookback),
					End:   cutoff,
				},
				Requirements:
					eventcap.ResearchRequirements{
						PITSafe: true,
						PreferredSourceTypes:
							[]eventcap.SourceType{
								eventcap.SourceOfficial,
								eventcap.SourceIndustryMedia,
								eventcap.SourceMainstreamMedia,
							},
						MinDocuments:          1,
						MinIndependentSources: 1,
					},
			},
			Query:
				"研究请求时点之前的近期公司公告、新闻与重要事件。",
		},
	}, nil
}
