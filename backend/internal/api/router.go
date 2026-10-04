// Package api builds the chi HTTP router and the handlers it dispatches to.
package api

import (
	"encoding/json"
	"net/http"
	"propertyfinder/internal/agent"
	"propertyfinder/internal/ai"
	"propertyfinder/internal/config"
	"propertyfinder/internal/events"
	"propertyfinder/internal/mcpserver"
	"propertyfinder/internal/service"
	"propertyfinder/internal/web"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
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
	note       *NoteHandler
	upload     *UploadHandler
	geocode    *GeocodeHandler
	chat       *ChatHandler
	ranking    *RankingHandler
	toolCall   *ToolCallHandler
	auth       *AuthHandler
	mcp        http.Handler
	dataDir    string
}

// NewServer constructs the API server, wiring handlers to services backed
// by pool. dataDir is the image storage volume root (ADR 0012), used by the
// listing service to clean up files on delete and served at /media/.
// appPassword and cookieSecure configure Frontend + API login (docs/adr/
// 0018): appPassword's SHA-256 digest signs session JWTs, cookieSecure
// sets the session cookie's Secure flag. provider is the resolved
// single-shot LLM provider (internal/ai) used by evaluate_listing (both
// directly, via EvaluationHandler, and through the Agent's
// evaluate_listing Tool — the same *EvaluationService instance backs
// both, Layer 6.5). agentProvider is the resolved multi-turn Provider
// (ADR 0007) used by the Agent loop — an unconfigured stand-in when
// llm.api_key is empty (agent.NewUnconfiguredProvider). broker fans out
// SSE progress for bulk re-evaluation (Layer 2.3) and chat streaming
// (Layer 6.3).
func NewServer(pool *pgxpool.Pool, llmConfig config.LLM, dataDir string, appPassword string, cookieSecure bool, provider ai.Provider, agentProvider agent.Provider, broker *events.Broker) *Server {
	evaluationService := service.NewEvaluationService(pool, provider, broker)
	rejectionAnalysisService := service.NewRejectionAnalysisService(pool, provider, broker)
	profileService := service.NewProfileService(pool)
	criterionService := service.NewCriterionService(pool)
	contextService := service.NewContextService(pool)
	mcpHandler := mcpserver.NewHandler(mcpserver.Deps{
		Profiles: profileService,
		Criteria: criterionService,
		Context:  contextService,
		NewRegistry: func(profileID uuid.UUID) *agent.Registry {
			return service.NewToolRegistry(pool, dataDir, evaluationService, profileID)
		},
	})
	return &Server{
		config:     &ConfigHandler{llm: llmConfig},
		profile:    &ProfileHandler{service: profileService},
		context:    &ContextHandler{service: contextService},
		criterion:  &CriterionHandler{service: criterionService},
		listing:    &ListingHandler{service: service.NewListingService(pool, dataDir, rejectionAnalysisService)},
		evaluation: &EvaluationHandler{service: evaluationService},
		events:     &EventHandler{broker: broker},
		image:      &ImageHandler{service: service.NewImageService(pool, dataDir)},
		note:       &NoteHandler{service: service.NewNoteService(pool)},
		upload:     &UploadHandler{service: service.NewUploadService(dataDir)},
		geocode:    &GeocodeHandler{},
		chat:       &ChatHandler{service: service.NewChatService(pool), agent: service.NewAgentService(pool, agentProvider, broker, llmConfig, dataDir, evaluationService)},
		ranking:    &RankingHandler{service: service.NewRankingService(pool, provider)},
		toolCall:   &ToolCallHandler{service: service.NewToolCallService(pool)},
		auth:       NewAuthHandler(appPassword, cookieSecure),
		mcp:        mcpHandler,
		dataDir:    dataDir,
	}
}

// Router builds the chi router. GET /health, POST /api/auth/login, and the
// embedded SPA catch-all stay public (docs/adr/0018); every other route is
// nested inside a protected group gated by server.auth.RequireAuth. GET
// /api/events and /media/* are mounted directly on protected, outside the
// Timeout middleware group nested further inside it — it's long-lived SSE,
// and chi's Timeout wrapper doesn't implement http.Flusher (would break
// streaming); /media/ is also outside it — file transfers shouldn't be cut
// off at 60s.
func (server *Server) Router() http.Handler {
	router := chi.NewRouter()
	router.Use(chimw.RequestID)
	router.Use(chimw.Logger)
	router.Use(chimw.Recoverer)

	router.Get("/health", healthHandler)
	router.Post("/api/auth/login", server.auth.Login)
	router.Handle("/mcp", server.auth.RequireBearer(server.mcp))

	// protected gates every route below behind RequireAuth (docs/adr/0018).
	protected := router.Group(func(r chi.Router) {
		r.Use(server.auth.RequireAuth)
	})

	protected.Get("/api/auth/status", server.auth.Status)
	protected.Post("/api/auth/logout", server.auth.Logout)
	protected.Get("/api/events", server.events.Stream)
	protected.Handle("/media/*", http.StripPrefix("/media/", http.FileServer(http.Dir(server.dataDir))))

	timed := protected.Group(func(r chi.Router) {
		r.Use(chimw.Timeout(60 * time.Second))
	})

	timed.Get("/api/config", server.config.get)
	timed.Post("/api/upload/pdf", server.upload.PDF)
	timed.Post("/api/upload/image", server.upload.Image)
	timed.Get("/api/geocode", server.geocode.Get)

	timed.Get("/api/context/global", server.context.GetGlobal)
	timed.Put("/api/context/global", server.context.PutGlobal)
	timed.Get("/api/context/global/history", server.context.GlobalHistory)
	timed.Post("/api/context/global/revisions/{revisionId}/restore", server.context.RestoreGlobal)

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
			r.Post("/context/revisions/{revisionId}/restore", server.context.RestoreProfile)

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
			r.Post("/chat/sessions/{sessionId}/stop", server.chat.StopTurn)
		})
	})

	timed.Route("/api/chat/messages/{messageId}/tool-calls/{callIdx}", func(r chi.Router) {
		r.Post("/confirm", server.toolCall.Confirm)
		r.Post("/reject", server.toolCall.Reject)
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
		r.Get("/grades", server.evaluation.GetGrades)

		r.Get("/images", server.image.List)
		r.Post("/images", server.image.Upload)
		r.Delete("/images/{imageId}", server.image.Delete)

		r.Get("/notes", server.note.List)
		r.Post("/notes", server.note.Create)
	})

	timed.Route("/api/notes/{noteId}", func(r chi.Router) {
		r.Patch("/", server.note.Update)
		r.Delete("/", server.note.Delete)
	})

	// Catch-all: embedded Vue SPA. Public — the login screen itself is
	// part of this bundle. Must be last — every /api/* and /media/*
	// route above takes precedence.
	router.HandleFunc("/*", web.SPAHandler)

	return router
}

func healthHandler(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(writer).Encode(map[string]string{"status": "ok"})
}
