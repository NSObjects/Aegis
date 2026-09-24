package main

import (
	"Aegis/internal/agent/market"
	"Aegis/internal/agent/technical"
	"Aegis/internal/agent/trading"
	technicalcap "Aegis/internal/capability/technical"
	"Aegis/internal/config"
	"Aegis/internal/llm"
	"Aegis/internal/research"
	"Aegis/internal/runtime"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"path/filepath"

	"github.com/cloudwego/eino/components/tool"
)

func main() {
	var (
		configPath  string
		skillsDir   string
		timeZone    string
		symbol      string
		marketQuery string
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
	flag.Parse()

	if err := run(
		context.Background(),
		configPath,
		skillsDir,
		timeZone,
		symbol,
		marketQuery,
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
) error {
	appCfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	models := llm.NewManager(
		appCfg.Models,
	)

	marketProfile, err :=
		appCfg.ModelProfileForAgent("market")
	if err != nil {
		return fmt.Errorf(
			"resolve market model profile: %w",
			err,
		)
	}

	technicalProfile, err :=
		appCfg.ModelProfileForAgent("technical")
	if err != nil {
		return fmt.Errorf(
			"resolve technical model profile: %w",
			err,
		)
	}

	tradingProfile, err :=
		appCfg.ModelProfileForAgent("trading")
	if err != nil {
		return fmt.Errorf(
			"resolve trading model profile: %w",
			err,
		)
	}

	marketModel, err := models.Get(
		ctx,
		marketProfile,
	)
	if err != nil {
		return fmt.Errorf(
			"create market model: %w",
			err,
		)
	}

	technicalModel, err := models.Get(
		ctx,
		technicalProfile,
	)
	if err != nil {
		return fmt.Errorf(
			"create technical model: %w",
			err,
		)
	}

	tradingModel, err := models.Get(
		ctx,
		tradingProfile,
	)
	if err != nil {
		return fmt.Errorf(
			"create trading model: %w",
			err,
		)
	}

	rt, err := runtime.New(
		ctx,
		runtime.Config{
			SkillsDir: skillsDir,
			TimeZone:  timeZone,
		},
	)
	if err != nil {
		return err
	}

	marketAgent, err := market.New(
		ctx,
		market.Config{
			Model:         marketModel,
			Handlers:      rt.Handlers(),
			MaxIterations: 30,
		},
	)
	if err != nil {
		return err
	}

	researcher, err := technicalcap.NewScriptResearcher(
		technicalcap.ScriptResearcherConfig{
			ScriptPath: filepath.Join(
				"internal",
				"capability",
				"technical",
				"scripts",
				"stock_research.py",
			),
		},
	)
	if err != nil {
		return fmt.Errorf(
			"create technical researcher: %w",
			err,
		)
	}

	researchTool, err := technicalcap.NewTool(researcher)
	if err != nil {
		return fmt.Errorf(
			"create technical research tool: %w",
			err,
		)
	}

	technicalAgent, err := technical.New(
		ctx,
		technical.Config{
			Model: technicalModel,
			Tools: []tool.BaseTool{
				researchTool,
			},
			Handlers:      rt.Handlers(),
			MaxIterations: 30,
		},
	)
	if err != nil {
		return err
	}

	tradingAgent, err := trading.New(
		ctx,
		trading.Config{
			Model:         tradingModel,
			MaxIterations: 30,
		},
	)
	if err != nil {
		return err
	}

	workflow, err := research.New(
		ctx,
		research.Config{
			MarketAgent:    marketAgent,
			TechnicalAgent: technicalAgent,
			TradingAgent:   tradingAgent,
		},
	)
	if err != nil {
		return err
	}

	result, err := workflow.Run(
		ctx,
		research.Input{
			Symbol:      symbol,
			MarketQuery: marketQuery,
		},
	)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(
		result,
		"",
		"  ",
	)
	if err != nil {
		return fmt.Errorf(
			"marshal research decision: %w",
			err,
		)
	}

	fmt.Println(string(data))

	return nil
}
