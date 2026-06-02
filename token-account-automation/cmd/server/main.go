package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/QuantumNous/new-api/token-account-automation/internal/config"
	automationdb "github.com/QuantumNous/new-api/token-account-automation/internal/db"
	"github.com/QuantumNous/new-api/token-account-automation/internal/executor"
	"github.com/QuantumNous/new-api/token-account-automation/internal/httpapi"
	"github.com/QuantumNous/new-api/token-account-automation/internal/queue"
	"github.com/QuantumNous/new-api/token-account-automation/internal/secret"
)

func main() {
	cfg := config.Load()
	database, err := automationdb.Open(cfg)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	if cfg.RunMigrations {
		if err := automationdb.Migrate(database); err != nil {
			log.Fatalf("migrate database: %v", err)
		}
	}

	queueService := queue.New(database)
	secretService := secret.New(database, cfg.SecretKey)
	api := httpapi.New(cfg, queueService, secretService)
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	if cfg.InternalExecutor {
		internalExecutor := executor.New(cfg, queueService, secretService, nil, log.Default())
		go func() {
			log.Printf("token-account-automation internal executor enabled worker_id=%s", cfg.InternalWorkerID)
			if err := internalExecutor.Run(appCtx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("internal executor stopped: %v", err)
			}
		}()
	}

	go func() {
		log.Printf("token-account-automation listening on %s", cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	appCancel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
