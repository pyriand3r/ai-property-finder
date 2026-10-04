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
	// Criteria backs propose_criteria's direct-write path.
	Criteria *service.CriterionService
	// Context backs update_context's direct-write path.
	Context *service.ContextService
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
// runTurn), plus list_profiles (bootstrap discovery) and direct-write
// variants of propose_criteria/update_context (see package doc). Callers
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
		Description: "Legt ein neues Bewertungskriterium an oder ändert ein bestehendes — schreibt sofort (über MCP gibt es keine Chat-Confirm-Card, anders als beim gleichnamigen internen Agent-Tool).",
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

	return mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{JSONResponse: true},
	)
}
