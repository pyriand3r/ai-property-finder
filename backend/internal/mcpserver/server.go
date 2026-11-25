// Package mcpserver exposes the existing internal Agent Tool catalog
// (internal/agent/tools.go) plus profile discovery over MCP's Streamable
// HTTP transport (docs/adr/0019), so external MCP clients (Claude Code,
// Claude Desktop via mcp-remote, opencode, omp) can drive listing
// ingestion/evaluation without an internal chat session.
package mcpserver

import (
	"net/http"
	"propertyfinder/internal/agent"
	"propertyfinder/internal/service"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Deps bundles the existing service instances and the per-call Registry
// constructor the MCP tool catalog needs — every field is a reference to
// the SAME instance backend/internal/api/router.go's NewServer already
// constructed for the REST API and the internal chat Agent, so MCP is a
// third consumer of the service layer (AGENTS.md "Layering"), not a
// second implementation of any of it.
type Deps struct {
	// Profiles backs list_profiles.
	Profiles *service.ProfileService
	// Criteria backs propose_criteria (writes proposed criteria only).
	Criteria *service.CriterionService
	// Context backs update_context's direct-write path.
	Context *service.ContextService
	// Evaluation backs get_evaluation_brief, submit_evaluation and
	// list_pending_evaluations (docs/adr/0026). Those tools never call the
	// LLM, so they work without an API key.
	Evaluation *service.EvaluationService
	// NewRegistry builds an agent.Registry scoped to profileID — the
	// caller (router.go's NewServer) passes a closure over
	// service.NewToolRegistry, its pool, dataDir, and evaluation service.
	NewRegistry func(profileID uuid.UUID) *agent.Registry
}

const serverName = "property-finder"

// serverVersion mirrors package.json's app version — the only version
// identifier this repo has (there is no separate backend version file).
const serverVersion = "0.0.0"

// NewHandler builds the MCP Streamable HTTP handler (docs/adr/0019)
// exposing internal/agent/tools.go's existing Tool catalog, each
// dispatched through the identical agent.Registry.Dispatch code path the
// internal chat Agent uses (backend/internal/service/agent_service.go's
// runTurn), plus list_profiles (bootstrap discovery), a proposal-only
// propose_criteria and a direct-write update_context (docs/adr/0024), and the
// external-evaluation tools get_evaluation_brief / submit_evaluation /
// list_pending_evaluations (docs/adr/0026). Callers
// must wrap the returned handler with an auth middleware
// (api.AuthHandler.RequireBearer) — this package has no opinion on
// authentication. JSONResponse:true makes every tools/call response a
// plain application/json body instead of an SSE stream — every tool here
// is a fast, single synchronous result, not an incremental one.
func NewHandler(deps Deps) http.Handler {
	srv := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: serverVersion}, nil)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_profiles",
		Description: "Listet alle Search Profiles mit ihrer UUID — als erster Aufruf, um eine profile_id für alle anderen Tools zu ermitteln.",
	}, listProfilesHandler(deps))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fetch_url",
		Description: "Lädt eine Webseite per HTTP GET und liefert den lesbaren Haupttext, alle gefundenen Bild-URLs und alle ausgehenden Links zurück.",
	}, fetchURLHandler(deps))

	mcp.AddTool(srv, &mcp.Tool{
		Name: "save_listing",
		Description: "Legt ein neues Inserat an oder aktualisiert ein bestehendes. listing_id weglassen = neu anlegen; " +
			"listing_id angeben = bestehendes Inserat aktualisieren. Felder, die in fields fehlen, bleiben unverändert. " +
			"Manuell bestätigte Felder (user_confirmed_fields) werden NICHT überschrieben, es sei denn sie stehen in force_overwrite.",
	}, saveListingHandler(deps))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "download_image",
		Description: "Lädt ein Bild von einer URL herunter und speichert es beim angegebenen Inserat. Das erste heruntergeladene Bild wird automatisch zum Titelbild, falls noch keins gesetzt ist.",
	}, downloadImageHandler(deps))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "evaluate_listing",
		Description: "Bewertet ein Inserat anhand der aktuellen Kriterien des Profils — vergibt Note A-F pro Kriterium und berechnet den Gesamt-Score.",
	}, evaluateListingHandler(deps))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "propose_criteria",
		Description: "Schlägt ein neues Bewertungskriterium vor (action=add) oder bessert einen noch offenen Vorschlag nach (action=edit). Das Kriterium hat Status 'proposed' und zählt bei Bewertung und Score erst, wenn der Nutzer es im Criteria-Tab freischaltet. Aktive Kriterien lassen sich über MCP nicht ändern; Namen sind pro Profil eindeutig.",
	}, proposeCriteriaHandler(deps))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "update_context",
		Description: "Hängt Text an den Globalen oder Profil-Kontext an — schreibt sofort (über MCP gibt es keine Chat-Confirm-Card, anders als beim gleichnamigen internen Agent-Tool).",
	}, updateContextHandler(deps))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "geocode",
		Description: "Löst eine Adresse in Koordinaten (Breitengrad/Längengrad) auf.",
	}, geocodeHandler(deps))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_profile_listings",
		Description: "Liefert eine Übersicht (Titel, Preis, Score, Status, Quell-URL) aller bestehenden Inserate in diesem Profil.",
	}, listProfileListingsHandler(deps))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "compute_ranking",
		Description: "Liefert die nach Score absteigend sortierte Rangliste aller bewerteten, nicht abgelehnten/archivierten Inserate im Profil.",
	}, computeRankingHandler(deps))

	mcp.AddTool(srv, &mcp.Tool{
		Name: "suggest_hazard_source",
		Description: "Schlägt eine öffentliche Naturgefahren-Datenquelle (wildfire, flood, earthquake, landslide) für ein Land vor, die der Quellenkatalog noch nicht enthält. " +
			"Der Vorschlag hat Status 'proposed': er ändert keine Bewertung und lädt nichts herunter. Der Nutzer gibt ihn im Globalen Kontext frei oder lehnt ihn ab. " +
			"Quellen, die schon im Katalog stehen oder schon vorgeschlagen wurden, werden abgelehnt.",
	}, suggestHazardSourceHandler(deps))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_pending_evaluations",
		Description: "Listet die Inserate eines Profils, die (neu) bewertet werden müssen (needs_reevaluation): id, title, status, overall_score (null, wenn noch nie bewertet).",
	}, listPendingEvaluationsHandler(deps))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_evaluation_brief",
		Description: "Liefert alles, um ein Inserat selbst zu bewerten: Bewertungsanweisung, aktive Kriterien (id, name, description, weight, is_subjective), Inserat samt Dokument, Globaler und Profil-Kontext, manuell bestätigte Felder und das Antwortformat. Vorgeschlagene (proposed) Kriterien sind nicht enthalten. Danach submit_evaluation aufrufen.",
	}, getEvaluationBriefHandler(deps))

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "submit_evaluation",
		Description: "Speichert die eigene Bewertung eines Inserats: genau eine Note (A-F) mit nicht-leerer Begründung pro aktivem Kriterium, identifiziert über criterion_id. Unbekannte, doppelte oder fehlende criterion_id wird abgelehnt und nichts gespeichert. Der Gesamt-Score wird vom Server berechnet.",
	}, submitEvaluationHandler(deps))

	return mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{JSONResponse: true},
	)
}
