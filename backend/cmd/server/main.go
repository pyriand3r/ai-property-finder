// Command server is the backend entrypoint.
//
// Layer 0 bootstrap order:
//  1. .env (optional)
//  2. Env config (DATABASE_URL required)
//  3. pgxpool connect
//  4. run pending migrations (forward-only; fail-fast on error)
//  5. seed Tier 2 app_config from env (write-once; DB authoritative after)
//  6. load Tier 2 app_config (llm.*)
//  7. start HTTP server (GET /health, GET /api/config)
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

	if seedError := config.SeedFromEnv(ctx, pool, envConfig.ClaudeAPIKey); seedError != nil {
		return seedError
	}
	llmConfig, err := config.LoadAll(ctx, pool)
	if err != nil {
		return err
	}
	log.Printf("llm config resolved: provider=%s model=%q api_key_configured=%t",
		llmConfig.Provider, llmConfig.Model, llmConfig.APIKey != "")

	router := api.NewServer(pool, llmConfig, envConfig.DataDir).Router()
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
	log.Print("shutdown complete")
	return nil
}
