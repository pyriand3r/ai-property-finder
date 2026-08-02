// Package api builds the chi HTTP router.
package api

import (
	"encoding/json"
	"net/http"
	"propertyfinder/internal/config"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

// NewRouter builds the chi router. Layer 0 wires health and the read-only
// config endpoint; later layers add profile/listing/chat/context routes as
// those domains land.
func NewRouter(llmConfig config.LLM) http.Handler {
	configHandler := &ConfigHandler{llm: llmConfig}

	router := chi.NewRouter()
	router.Use(chimw.RequestID)
	router.Use(chimw.Logger)
	router.Use(chimw.Recoverer)
	router.Use(chimw.Timeout(60 * time.Second))

	router.Get("/health", healthHandler)
	router.Get("/api/config", configHandler.get)

	return router
}

func healthHandler(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(writer).Encode(map[string]string{"status": "ok"})
}
