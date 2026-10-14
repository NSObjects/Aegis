package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
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
	dev := flag.Bool("dev", false, "local-only debug mode without API authentication")
	path := flag.String("store", ".aegis/runs", "single-process run store")
	credentialsPath := flag.String("credential-store", ".aegis/credentials.enc", "encrypted single-operator provider credentials")
	cfg := flag.String("config", "configs/config.yaml", "model configuration")
	skills := flag.String("skills", "skills", "runtime skills")
	zone := flag.String("timezone", "Asia/Shanghai", "research timezone")
	evidence := flag.String("evidence-store", ".aegis/evidence", "event evidence snapshots")
	parallel := flag.Int("max-parallel", 1, "simultaneous research workflows")
	maxActive := flag.Int("max-active", 4, "running plus queued research")
	runTimeout := flag.Duration("run-timeout", 20*time.Minute, "maximum wall-clock time for a queued or running research task")
	lookback := flag.Duration("event-lookback", 7*24*time.Hour, "live event evidence lookback")
	full := flag.Bool("portfolio-review", true, "full research with explicit account state")
	flag.Parse()
	if *dev {
		host, _, err := net.SplitHostPort(*addr)
		ip := net.ParseIP(host)
		if err != nil || ip == nil || !ip.IsLoopback() {
			log.Fatal("-dev requires literal loopback listen address (127.0.0.1 or ::1)")
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	token := os.Getenv("AEGIS_WEB_API_TOKEN")
	mode := application.ModeDecision
	if *full {
		mode = application.ModePortfolio
	}
	masterKey := os.Getenv("AEGIS_CREDENTIAL_MASTER_KEY")
	if *dev && masterKey == "" {
		var err error
		masterKey, err = localDevMasterKey(filepath.Join(filepath.Dir(*credentialsPath), ".local-dev-master.key"))
		if err != nil {
			log.Fatal(err)
		}
	}
	credentialStore, err := settings.NewStore(*credentialsPath, masterKey)
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
	if err := runs.SetRunTimeout(*runTimeout); err != nil { log.Fatal(err) }
	// Bound process shutdown even when a provider ignores cancellation. This
	// does not terminate a stuck worker or free its slot prematurely; the
	// process is exiting after the grace period.
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := runs.CloseContext(shutdownCtx); err != nil {
			log.Printf("AEGIS_SHUTDOWN_WARNING component=run_manager category=EXECUTOR_NOT_DRAINED")
		}
	}()
	var handler *httpapi.Handler
	if *dev {
		handler, err = httpapi.NewLocalDevHandler(runs, mode, *lookback)
	} else {
		handler, err = httpapi.NewHandler(runs, token, mode, *lookback)
	}
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

// localDevMasterKey generates one persistent key for a single local workspace.
// The colocated key is a usability measure, not protection from a compromised host.
func localDevMasterKey(path string) (string, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return "", errors.New("local key directory is not a regular directory")
	}
	if info.Mode().Perm()&0077 != 0 {
		if err := os.Chmod(dir, 0700); err != nil {
			return "", err
		}
	}
	info, err = os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return "", errors.New("local key file has unsafe permissions or type")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		key := strings.TrimSpace(string(data))
		raw, err := hex.DecodeString(key)
		if err != nil || len(raw) != 32 {
			return "", errors.New("invalid persisted local key")
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	value := hex.EncodeToString(key)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return localDevMasterKey(path)
		}
		return "", err
	}
	_, writeErr := fmt.Fprintln(file, value)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(path)
		return "", writeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return value, nil
}
