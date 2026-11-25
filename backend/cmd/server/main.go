// Command server is the backend entrypoint.
//
// Layer 0 bootstrap order:
//  1. .env (optional)
//  2. Env config (DATABASE_URL required)
//  3. pgxpool connect
//  4. run pending migrations (forward-only; fail-fast on error)
//  5. sync the embedded natural-hazard datasets into hazard_* (idempotent;
//     fail-fast; docs/adr/0025)
//  6. resume portal searches that were running at the last shutdown
//     (docs/adr/0028)
//  7. seed Tier 2 app_config from env (write-once; DB authoritative after)
//  8. load Tier 2 app_config (llm.*), build the live LLM runtime from it
//     (internal/llm; editable at runtime via PATCH /api/config)
//  9. settle saved search runs the last shutdown interrupted, start the
//     scheduler (docs/adr/0029)
//  10. start HTTP server (GET /health, GET/PATCH /api/config, GET /api/events, ...)
//
// The rest of the API surface lands in later layers.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"propertyfinder/internal/api"
	"propertyfinder/internal/config"
	"propertyfinder/internal/db"
	"propertyfinder/internal/events"
	"propertyfinder/internal/fetch"
	"propertyfinder/internal/llm"
	"propertyfinder/internal/service"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("startup: %v", err)
	}
}

// run performs the bootstrap sequence and blocks until shutdown. It returns
// (rather than calling os.Exit directly) so deferred cleanup — closing the
// connection pool — always runs, even on a startup failure partway through.
func run() error {
	// .env is optional — Docker Compose injects env directly; a missing file
	// is fine, an unreadable one is worth a warning before the logger exists.
	if loadError := godotenv.Load(); loadError != nil && !os.IsNotExist(loadError) {
		log.Printf("warning: .env present but unreadable: %v", loadError)
	}

	envConfig, err := config.LoadEnv(os.Environ())
	if err != nil {
		return err
	}

	ctx := context.Background()
	pool, err := config.ConnectPool(ctx, envConfig.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if migrationError := db.Run(ctx, envConfig.DatabaseURL); migrationError != nil {
		return migrationError
	}

	if hazardError := service.NewHazardService(pool).SeedStatic(ctx); hazardError != nil {
		return hazardError
	}
	log.Print("hazard datasets synced")

	resumed, recoverError := service.NewPortalSearchService(pool, fetch.NewClient()).Recover(ctx)
	if recoverError != nil {
		return recoverError
	}
	log.Printf("portal searches resumed: %d", resumed)

	if seedError := config.SeedFromEnv(ctx, pool, envConfig.ClaudeAPIKey); seedError != nil {
		return seedError
	}
	llmConfig, err := config.LoadAll(ctx, pool)
	if err != nil {
		return err
	}
	log.Printf("llm config resolved: provider=%s model=%q api_key_configured=%t",
		llmConfig.Provider, llmConfig.Model, llmConfig.APIKey != "")

	llmRuntime, err := llm.New(llmConfig, llm.PoolStore{Pool: pool})
	if err != nil {
		return err
	}

	broker := events.NewBroker()
	defer broker.Stop()

	apiServer := api.NewServer(pool, llmRuntime, envConfig.DataDir, envConfig.AppPassword, envConfig.CookieSecure, broker)
	interrupted, recoverError := apiServer.RecoverSavedSearches(ctx)
	if recoverError != nil {
		return recoverError
	}
	log.Printf("saved search runs interrupted by the last shutdown: %d", interrupted)

	schedulerCtx, stopScheduler := context.WithCancel(ctx)
	defer stopScheduler()
	schedulerDone := make(chan struct{})
	go func() {
		defer close(schedulerDone)
		apiServer.RunScheduler(schedulerCtx)
	}()

	router := apiServer.Router()
	server := &http.Server{
		Addr:              envConfig.HTTPListenAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("http server listening on %s", envConfig.HTTPListenAddr)
		if listenError := server.ListenAndServe(); listenError != nil && !errors.Is(listenError, http.ErrServerClosed) {
			serverErrors <- listenError
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	select {
	case listenError := <-serverErrors:
		return listenError
	case <-shutdown:
		log.Print("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if shutdownError := server.Shutdown(shutdownCtx); shutdownError != nil {
		log.Printf("http server shutdown: %v", shutdownError)
	}
	stopScheduler()
	<-schedulerDone
	log.Print("shutdown complete")
	return nil
}
