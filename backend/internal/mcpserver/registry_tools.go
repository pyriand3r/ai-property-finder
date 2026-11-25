package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"propertyfinder/internal/agent"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// dispatchViaRegistry marshals input, builds a Registry scoped to
// profileIDRaw via deps.NewRegistry, dispatches toolName through
// agent.Registry.Dispatch — the exact code path the internal chat Agent
// uses — and decodes the JSON result. Extra fields in the marshaled input
// beyond what a given tool's private arg-decode struct expects (e.g.
// profile_id, which none of these 7 tools' own decoders declare) are
// silently ignored by encoding/json, matching how internal/agent/tools.go
// already decodes call.Arguments. A Dispatch isError result becomes a Go
// error carrying the "error" field's message — mcp.AddTool's generic
// wrapper then reports it as a tool error (CallToolResult.IsError=true)
// automatically.
func dispatchViaRegistry(ctx context.Context, deps Deps, profileIDRaw, toolName string, input any) (any, error) {
	profileID, err := uuid.Parse(profileIDRaw)
	if err != nil {
		return nil, fmt.Errorf("invalid profile_id: %w", err)
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode arguments: %w", err)
	}
	registry := deps.NewRegistry(profileID)
	result := registry.Dispatch(ctx, agent.ToolCall{Name: toolName, Arguments: raw})

	var output any
	if err := json.Unmarshal([]byte(result.Result), &output); err != nil {
		return nil, fmt.Errorf("decode %s result: %w", toolName, err)
	}
	if result.IsError {
		message := toolName + " failed"
		if obj, ok := output.(map[string]any); ok {
			if msg, ok := obj["error"].(string); ok {
				message = msg
			}
		}
		return nil, errors.New(message)
	}
	return output, nil
}

// FetchURLInput mirrors fetch_url's schema in internal/agent/tools.go.
// profile_id is unused by this specific tool but required for a uniform
// schema across every property-finder MCP tool.
type FetchURLInput struct {
	ProfileID string `json:"profile_id" jsonschema:"UUID of the Search Profile (call list_profiles first if unknown)"`
	URL       string `json:"url" jsonschema:"Absolute URL of the page to fetch"`
}

func fetchURLHandler(deps Deps) mcp.ToolHandlerFor[FetchURLInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input FetchURLInput) (*mcp.CallToolResult, any, error) {
		output, err := dispatchViaRegistry(ctx, deps, input.ProfileID, "fetch_url", input)
		return nil, output, err
	}
}

// SaveListingFieldsInput mirrors save_listing's "fields" object exactly
// (internal/agent/tools.go's saveListingFields).
type SaveListingFieldsInput struct {
	Title          *string  `json:"title,omitempty"`
	SourceURL      *string  `json:"source_url,omitempty"`
	Price          *float64 `json:"price,omitempty"`
	PriceCurrency  *string  `json:"price_currency,omitempty" jsonschema:"Currency code of the source, e.g. SEK, EUR — do not convert"`
	PropertySizeM2 *float64 `json:"property_size_m2,omitempty"`
	PlotSizeM2     *float64 `json:"plot_size_m2,omitempty"`
	Address        *string  `json:"address,omitempty"`
	Region         *string  `json:"region,omitempty"`
	Country        *string  `json:"country,omitempty"`
	Latitude       *float64 `json:"latitude,omitempty"`
	Longitude      *float64 `json:"longitude,omitempty"`
	Description    *string  `json:"description,omitempty"`
	DocumentMD     *string  `json:"document_md,omitempty" jsonschema:"Original text excerpt, German translation, evaluation table, pros/cons"`
	SellerName     *string  `json:"seller_name,omitempty"`
	SellerPhone    *string  `json:"seller_phone,omitempty"`
	SellerEmail    *string  `json:"seller_email,omitempty"`
	SellerNotes    *string  `json:"seller_notes,omitempty"`
}

// SaveListingInput mirrors save_listing's schema in internal/agent/tools.go.
type SaveListingInput struct {
	ProfileID      string                 `json:"profile_id" jsonschema:"UUID of the Search Profile this listing belongs to"`
	ListingID      *string                `json:"listing_id,omitempty" jsonschema:"UUID of an existing listing to update; omit to create a new one"`
	Fields         SaveListingFieldsInput `json:"fields" jsonschema:"Structured fields; keys omitted from a previous call are left unchanged"`
	ForceOverwrite []string               `json:"force_overwrite,omitempty" jsonschema:"Field names to overwrite even though the user already manually confirmed them"`
}

func saveListingHandler(deps Deps) mcp.ToolHandlerFor[SaveListingInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input SaveListingInput) (*mcp.CallToolResult, any, error) {
		output, err := dispatchViaRegistry(ctx, deps, input.ProfileID, "save_listing", input)
		return nil, output, err
	}
}

// DownloadImageInput mirrors download_image's schema.
type DownloadImageInput struct {
	ProfileID string  `json:"profile_id" jsonschema:"UUID of the Search Profile (unused by this tool but required for schema uniformity)"`
	ListingID string  `json:"listing_id" jsonschema:"UUID of the listing to attach the image to"`
	ImageURL  string  `json:"image_url" jsonschema:"Absolute URL of the image"`
	Category  *string `json:"category,omitempty" jsonschema:"One of: exterior, interior, floor_plan, outbuilding, other"`
}

func downloadImageHandler(deps Deps) mcp.ToolHandlerFor[DownloadImageInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input DownloadImageInput) (*mcp.CallToolResult, any, error) {
		output, err := dispatchViaRegistry(ctx, deps, input.ProfileID, "download_image", input)
		return nil, output, err
	}
}

// EvaluateListingInput mirrors evaluate_listing's schema.
type EvaluateListingInput struct {
	ProfileID string `json:"profile_id" jsonschema:"UUID of the Search Profile (unused by this tool but required for schema uniformity)"`
	ListingID string `json:"listing_id" jsonschema:"UUID of the listing to evaluate"`
}

func evaluateListingHandler(deps Deps) mcp.ToolHandlerFor[EvaluateListingInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input EvaluateListingInput) (*mcp.CallToolResult, any, error) {
		output, err := dispatchViaRegistry(ctx, deps, input.ProfileID, "evaluate_listing", input)
		return nil, output, err
	}
}

// GeocodeInput mirrors geocode's schema.
type GeocodeInput struct {
	ProfileID string `json:"profile_id" jsonschema:"UUID of the Search Profile (unused by this tool but required for schema uniformity)"`
	Address   string `json:"address" jsonschema:"Address to resolve"`
	Country   string `json:"country,omitempty" jsonschema:"ISO 3166-1 alpha-2 country code to narrow the search, e.g. SE, AT, PT"`
}

func geocodeHandler(deps Deps) mcp.ToolHandlerFor[GeocodeInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input GeocodeInput) (*mcp.CallToolResult, any, error) {
		output, err := dispatchViaRegistry(ctx, deps, input.ProfileID, "geocode", input)
		return nil, output, err
	}
}

// SuggestHazardSourceInput mirrors suggest_hazard_source's schema in
// internal/agent/tools.go. The suggestion is global, not per profile;
// profile_id is only needed because the Registry is profile-scoped.
type SuggestHazardSourceInput struct {
	ProfileID  string `json:"profile_id" jsonschema:"UUID of the Search Profile (unused by this tool but required for schema uniformity)"`
	Country    string `json:"country" jsonschema:"ISO 3166-1 alpha-2 country code, e.g. SE, AT, PT"`
	HazardType string `json:"hazard_type" jsonschema:"One of: wildfire, flood, earthquake, landslide"`
	Scope      string `json:"scope,omitempty" jsonschema:"Part of the country the source covers, e.g. Azores; empty = whole country"`
	Title      string `json:"title" jsonschema:"Name of the dataset"`
	Publisher  string `json:"publisher" jsonschema:"Authority or institution that publishes it"`
	URL        string `json:"url" jsonschema:"Absolute http(s) URL of the dataset page or download"`
	License    string `json:"license,omitempty" jsonschema:"License, if known"`
	Rationale  string `json:"rationale" jsonschema:"Why it is useful: coverage, resolution (municipality level?), format"`
}

func suggestHazardSourceHandler(deps Deps) mcp.ToolHandlerFor[SuggestHazardSourceInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input SuggestHazardSourceInput) (*mcp.CallToolResult, any, error) {
		output, err := dispatchViaRegistry(ctx, deps, input.ProfileID, "suggest_hazard_source", input)
		return nil, output, err
	}
}

// SearchPortalInput mirrors search_portal's schema in internal/agent/tools.go.
// Start a search with url, read a running one back with task_id.
type SearchPortalInput struct {
	ProfileID    string `json:"profile_id" jsonschema:"UUID of the Search Profile (call list_profiles first if unknown)"`
	URL          string `json:"url,omitempty" jsonschema:"Absolute URL of the portal's search-result page; starts a search"`
	TaskID       string `json:"task_id,omitempty" jsonschema:"UUID of a running search from an earlier call; use instead of url to read its result"`
	MaxPages     int    `json:"max_pages,omitempty" jsonschema:"Number of result pages, 1-5, default 1"`
	PageParam    string `json:"page_param,omitempty" jsonschema:"Name of the query parameter that holds the page number; required when max_pages > 1"`
	LinkContains string `json:"link_contains,omitempty" jsonschema:"Only keep links that contain this text"`
}

func searchPortalHandler(deps Deps) mcp.ToolHandlerFor[SearchPortalInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input SearchPortalInput) (*mcp.CallToolResult, any, error) {
		output, err := dispatchViaRegistry(ctx, deps, input.ProfileID, "search_portal", input)
		return nil, output, err
	}
}

// ProfileScopedInput is shared by list_profile_listings and
// compute_ranking, neither of which takes any field besides profile_id
// (internal/agent/tools.go's Dispatch doesn't even read call.Arguments
// for these two tool names).
type ProfileScopedInput struct {
	ProfileID string `json:"profile_id" jsonschema:"UUID of the Search Profile"`
}

func listProfileListingsHandler(deps Deps) mcp.ToolHandlerFor[ProfileScopedInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input ProfileScopedInput) (*mcp.CallToolResult, any, error) {
		output, err := dispatchViaRegistry(ctx, deps, input.ProfileID, "list_profile_listings", input)
		return nil, output, err
	}
}

func computeRankingHandler(deps Deps) mcp.ToolHandlerFor[ProfileScopedInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input ProfileScopedInput) (*mcp.CallToolResult, any, error) {
		output, err := dispatchViaRegistry(ctx, deps, input.ProfileID, "compute_ranking", input)
		return nil, output, err
	}
}
