package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"Aegis/internal/application"
	"Aegis/internal/httpapi"
	"Aegis/internal/settings"
)

// liveExecutor resolves the latest encrypted provider credentials for each
// new research run. It does not mutate global environment variables or switch
// credentials on an already-running workflow.
type liveExecutor struct {
	options     application.LiveServiceOptions
	credentials *settings.Store
}

func (e *liveExecutor) Run(ctx context.Context, request application.ResearchRequest) (*application.ResearchResponse, error) {
	values, err := e.credentials.Load(ctx)
	if err != nil {
		return nil, err
	}
	opts := e.options
	opts.Credentials = values
	service, err := application.NewLiveResearchService(ctx, opts)
	if err != nil {
		return nil, err
	}
	return service.Run(ctx, request)
}

func main() {
	addr := flag.String("listen", "127.0.0.1:8686", "bind address (loopback by default)")
	path := flag.String("store", ".aegis/runs", "single-process run store")
	credentialsPath := flag.String("credential-store", ".aegis/credentials.enc", "encrypted single-operator provider credentials")
	cfg := flag.String("config", "configs/config.yaml", "model configuration")
	skills := flag.String("skills", "skills", "runtime skills")
	zone := flag.String("timezone", "Asia/Shanghai", "research timezone")
	evidence := flag.String("evidence-store", ".aegis/evidence", "event evidence snapshots")
	parallel := flag.Int("max-parallel", 1, "simultaneous research workflows")
	maxActive := flag.Int("max-active", 4, "running plus queued research")
	lookback := flag.Duration("event-lookback", 7*24*time.Hour, "live event evidence lookback")
	full := flag.Bool("portfolio-review", true, "full research with explicit account state")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	token := os.Getenv("AEGIS_WEB_API_TOKEN")
	mode := application.ModeDecision
	if *full {
		mode = application.ModePortfolio
	}
	credentialStore, err := settings.NewStore(*credentialsPath, os.Getenv("AEGIS_CREDENTIAL_MASTER_KEY"))
	if err != nil {
		log.Fatal(err)
	}
	executor := &liveExecutor{options: application.LiveServiceOptions{
		ConfigPath: *cfg, SkillsDir: *skills, TimeZone: *zone, EvidenceStore: *evidence,
		EventLookback: *lookback, PortfolioReview: *full,
	}, credentials: credentialStore}
	store, err := application.NewFileRunStore(*path)
	if err != nil {
		log.Fatal(err)
	}
	runs, err := application.NewRunManager(executor, store, *parallel, *maxActive)
	if err != nil {
		log.Fatal(err)
	}
	defer runs.Close()
	handler, err := httpapi.NewHandler(runs, token, mode, *lookback)
	if err != nil {
		log.Fatal(err)
	}
	handler.WithCredentialStore(credentialStore)
	server := &http.Server{Addr: *addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, MaxHeaderBytes: 16 << 10}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("Aegis research API listening at %s (loopback by default)", *addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
