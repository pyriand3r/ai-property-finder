// Package api builds the chi HTTP router and the handlers it dispatches to.
package api

import (
	"encoding/json"
	"net/http"
	"propertyfinder/internal/config"
	"propertyfinder/internal/service"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Server wraps the chi router and the configured handlers.
type Server struct {
	config    *ConfigHandler
	profile   *ProfileHandler
	context   *ContextHandler
	criterion *CriterionHandler
}

// NewServer constructs the API server, wiring handlers to services backed
// by pool.
func NewServer(pool *pgxpool.Pool, llmConfig config.LLM) *Server {
	return &Server{
		config:    &ConfigHandler{llm: llmConfig},
		profile:   &ProfileHandler{service: service.NewProfileService(pool)},
		context:   &ContextHandler{service: service.NewContextService(pool)},
		criterion: &CriterionHandler{service: service.NewCriterionService(pool)},
	}
}

// Router builds the chi router.
func (server *Server) Router() http.Handler {
	router := chi.NewRouter()
	router.Use(chimw.RequestID)
	router.Use(chimw.Logger)
	router.Use(chimw.Recoverer)
	router.Use(chimw.Timeout(60 * time.Second))

	router.Get("/health", healthHandler)
	router.Get("/api/config", server.config.get)

	router.Get("/api/context/global", server.context.GetGlobal)
	router.Put("/api/context/global", server.context.PutGlobal)

	router.Route("/api/profiles", func(r chi.Router) {
		r.Get("/", server.profile.List)
		r.Post("/", server.profile.Create)
		r.Route("/{profileId}", func(r chi.Router) {
			r.Get("/", server.profile.Get)
			r.Patch("/", server.profile.Update)
			r.Delete("/", server.profile.Delete)
			r.Patch("/archive", server.profile.Archive)

			r.Get("/context", server.context.GetProfile)
			r.Put("/context", server.context.PutProfile)
			r.Get("/context/history", server.context.History)

			r.Get("/criteria", server.criterion.List)
			r.Post("/criteria", server.criterion.Create)
		})
	})

	router.Route("/api/criteria/{criterionId}", func(r chi.Router) {
		r.Patch("/", server.criterion.Update)
		r.Delete("/", server.criterion.Delete)
	})

	return router
}

func healthHandler(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(writer).Encode(map[string]string{"status": "ok"})
}
