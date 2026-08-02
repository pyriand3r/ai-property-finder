// Package api builds the chi HTTP router and the handlers it dispatches to.
package api

import (
	"encoding/json"
	"net/http"
	"propertyfinder/internal/ai"
	"propertyfinder/internal/config"
	"propertyfinder/internal/events"
	"propertyfinder/internal/service"
	"propertyfinder/internal/web"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Server wraps the chi router and the configured handlers.
type Server struct {
	config     *ConfigHandler
	profile    *ProfileHandler
	context    *ContextHandler
	criterion  *CriterionHandler
	listing    *ListingHandler
	evaluation *EvaluationHandler
	events     *EventHandler
	image      *ImageHandler
	upload     *UploadHandler
	geocode    *GeocodeHandler
	chat       *ChatHandler
	ranking    *RankingHandler
	dataDir    string
}

// NewServer constructs the API server, wiring handlers to services backed
// by pool. dataDir is the image storage volume root (ADR 0012), used by the
// listing service to clean up files on delete and served at /media/.
// provider is the resolved single-shot LLM provider (internal/ai) used by
// evaluate_listing. broker fans out SSE progress for bulk re-evaluation
// (Layer 2.3) and, later, chat streaming (Layer 6.3).
func NewServer(pool *pgxpool.Pool, llmConfig config.LLM, dataDir string, provider ai.Provider, broker *events.Broker) *Server {
	return &Server{
		config:     &ConfigHandler{llm: llmConfig},
		profile:    &ProfileHandler{service: service.NewProfileService(pool)},
		context:    &ContextHandler{service: service.NewContextService(pool)},
		criterion:  &CriterionHandler{service: service.NewCriterionService(pool)},
		listing:    &ListingHandler{service: service.NewListingService(pool, dataDir)},
		evaluation: &EvaluationHandler{service: service.NewEvaluationService(pool, provider, broker)},
		events:     &EventHandler{broker: broker},
		image:      &ImageHandler{service: service.NewImageService(pool, dataDir)},
		upload:     &UploadHandler{service: service.NewUploadService(dataDir)},
		geocode:    &GeocodeHandler{},
		chat:       &ChatHandler{service: service.NewChatService(pool)},
		ranking:    &RankingHandler{service: service.NewRankingService(pool, provider)},
		dataDir:    dataDir,
	}
}

// Router builds the chi router. GET /api/events is mounted outside the
// Timeout middleware group — it's long-lived SSE, and chi's Timeout
// wrapper doesn't implement http.Flusher (would break streaming). /media/
// is also outside it — file transfers shouldn't be cut off at 60s.
func (server *Server) Router() http.Handler {
	router := chi.NewRouter()
	router.Use(chimw.RequestID)
	router.Use(chimw.Logger)
	router.Use(chimw.Recoverer)

	router.Get("/health", healthHandler)
	router.Get("/api/events", server.events.Stream)
	router.Handle("/media/*", http.StripPrefix("/media/", http.FileServer(http.Dir(server.dataDir))))

	timed := router.Group(func(r chi.Router) {
		r.Use(chimw.Timeout(60 * time.Second))
	})

	timed.Get("/api/config", server.config.get)
	timed.Post("/api/upload/pdf", server.upload.PDF)
	timed.Get("/api/geocode", server.geocode.Get)

	timed.Get("/api/context/global", server.context.GetGlobal)
	timed.Put("/api/context/global", server.context.PutGlobal)

	timed.Route("/api/profiles", func(r chi.Router) {
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

			r.Get("/listings", server.listing.List)
			r.Post("/listings", server.listing.Create)
			r.Post("/listings/reevaluate-all", server.evaluation.ReevaluateAll)
			r.Post("/ranking/refresh", server.ranking.Refresh)

			r.Get("/chat/sessions", server.chat.ListSessions)
			r.Post("/chat/sessions", server.chat.StartSession)
			r.Get("/chat/sessions/{sessionId}/messages", server.chat.ListMessages)
			r.Post("/chat/sessions/{sessionId}/messages", server.chat.SendMessage)
		})
	})

	timed.Route("/api/criteria/{criterionId}", func(r chi.Router) {
		r.Patch("/", server.criterion.Update)
		r.Delete("/", server.criterion.Delete)
	})

	timed.Route("/api/listings/{listingId}", func(r chi.Router) {
		r.Get("/", server.listing.Get)
		r.Patch("/", server.listing.Update)
		r.Delete("/", server.listing.Delete)
		r.Post("/evaluate", server.evaluation.Evaluate)

		r.Get("/images", server.image.List)
		r.Post("/images", server.image.Upload)
		r.Delete("/images/{imageId}", server.image.Delete)
	})

	// Catch-all: embedded Vue SPA. Must be last — every /api/* and /media/*
	// route above takes precedence.
	router.HandleFunc("/*", web.SPAHandler)

	return router
}

func healthHandler(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(writer).Encode(map[string]string{"status": "ok"})
}
