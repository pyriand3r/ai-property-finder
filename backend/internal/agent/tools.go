package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"propertyfinder/internal/domain"
	"propertyfinder/internal/fetch"
	"propertyfinder/internal/geocode"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Tools returns the full Tool catalog offered to the model: reading and
// saving a listing plus its photos (Layer 6.4), evaluating it against the
// profile's Criteria and proposing Criteria/Context changes for the user
// to confirm (Layer 6.5), and address lookup, cross-listing comparison,
// and ranking (Layer 6.6, spec 00-03 "Tool Catalog"). read_pdf isn't a
// dispatchable Tool — an uploaded PDF is attached to the triggering
// message automatically (see AgentService.runTurn), matching how the
// model receives it: as native document content, not a tool result.
func Tools() []Tool {
	return []Tool{
		{
			Name:        "fetch_url",
			Description: "Lädt eine Webseite per HTTP GET und liefert den lesbaren Haupttext, alle gefundenen Bild-URLs und alle ausgehenden Links zurück. Nutze dies für die Hauptseite eines Inserats und für sekundäre Links, denen eine Portal-Regel folgt (z.B. \"Läs mer hos mäklaren\").",
			InputSchema: json.RawMessage(`{
				"properties": {
					"url": {"type": "string", "description": "Absolute URL der zu ladenden Seite."}
				},
				"required": ["url"]
			}`),
		},
		{
			Name: "save_listing",
			Description: "Legt ein neues Inserat an oder aktualisiert ein bestehendes. listing_id weglassen = neu anlegen; " +
				"listing_id angeben = bestehendes Inserat aktualisieren. Felder, die in fields fehlen, bleiben unverändert. " +
				"Manuell bestätigte Felder (user_confirmed_fields) werden NICHT überschrieben, es sei denn sie stehen in " +
				"force_overwrite — das nur nach ausdrücklicher Zustimmung des Nutzers im Chat setzen.",
			InputSchema: json.RawMessage(`{
				"properties": {
					"listing_id": {"type": "string", "description": "UUID eines bestehenden Inserats zum Aktualisieren. Weglassen für Neuanlage."},
					"fields": {
						"type": "object",
						"description": "Strukturierte Felder. Fehlende Schlüssel bleiben unverändert.",
						"properties": {
							"title": {"type": "string"},
							"source_url": {"type": "string"},
							"price": {"type": "number"},
							"price_currency": {"type": "string", "description": "Währungscode der Quelle, z.B. SEK, EUR — nicht umrechnen."},
							"property_size_m2": {"type": "number"},
							"plot_size_m2": {"type": "number"},
							"address": {"type": "string"},
							"region": {"type": "string"},
							"country": {"type": "string"},
							"latitude": {"type": "number"},
							"longitude": {"type": "number"},
							"description": {"type": "string"},
							"document_md": {"type": "string", "description": "Originaltext-Auszug, deutsche Übersetzung, Bewertungstabelle, Vor-/Nachteile — siehe Systemanweisung."},
							"seller_name": {"type": "string"},
							"seller_phone": {"type": "string"},
							"seller_email": {"type": "string"},
							"seller_notes": {"type": "string"}
						}
					},
					"force_overwrite": {
						"type": "array",
						"items": {"type": "string"},
						"description": "Feldnamen, die trotz manueller Bestätigung überschrieben werden sollen — nur nach expliziter Zustimmung des Nutzers."
					}
				},
				"required": ["fields"]
			}`),
		},
		{
			Name:        "download_image",
			Description: "Lädt ein Bild von einer URL herunter und speichert es beim angegebenen Inserat. Das erste heruntergeladene Bild wird automatisch zum Titelbild, falls noch keins gesetzt ist.",
			InputSchema: json.RawMessage(`{
				"properties": {
					"listing_id": {"type": "string", "description": "UUID des Inserats, zu dem das Bild gehört."},
					"image_url": {"type": "string", "description": "Absolute URL des Bildes."},
					"category": {"type": "string", "enum": ["exterior", "interior", "floor_plan", "outbuilding", "other"]}
				},
				"required": ["listing_id", "image_url"]
			}`),
		},
		{
			Name:        "evaluate_listing",
			Description: "Bewertet ein Inserat anhand der aktuellen Kriterien des Profils — vergibt Note A-F pro Kriterium und berechnet den Gesamt-Score. Nur aufrufen wenn das Profil bereits bestätigte Kriterien hat; falls nicht, zuerst propose_criteria nutzen.",
			InputSchema: json.RawMessage(`{
				"properties": {
					"listing_id": {"type": "string", "description": "UUID des zu bewertenden Inserats."}
				},
				"required": ["listing_id"]
			}`),
		},
		{
			Name: "propose_criteria",
			Description: "Schlägt ein neues oder geändertes Bewertungskriterium vor. Schreibt NICHT direkt in die Datenbank — " +
				"der Nutzer muss im Chat über eine Confirm-Card bestätigen. Nutze dies beim ersten Inserat eines Profils ohne " +
				"Kriterien, oder wenn der Nutzer mitten im Gespräch ein neues Bewertungskriterium erwähnt oder ein bestehendes ändern möchte.",
			InputSchema: json.RawMessage(`{
				"properties": {
					"action": {"type": "string", "enum": ["add", "edit"], "description": "add = neues Kriterium, edit = bestehendes ändern."},
					"criterion_id": {"type": "string", "description": "UUID des zu ändernden Kriteriums. Erforderlich bei action=edit."},
					"name": {"type": "string"},
					"description": {"type": "string"},
					"is_subjective": {"type": "boolean", "description": "true = subjektiv (nur mit LLM-Einschätzung bewertbar), false = objektiv (direkt aus Daten ablesbar)."},
					"weight": {"type": "integer", "description": "1-5, Standard 1."}
				},
				"required": ["action", "name"]
			}`),
		},
		{
			Name: "update_context",
			Description: "Schlägt eine Ergänzung des Globalen oder Profil-Kontexts vor (z.B. eine neu gelernte Portal-Regel). " +
				"Schreibt NICHT direkt — der Nutzer muss im Chat über eine Confirm-Card bestätigen.",
			InputSchema: json.RawMessage(`{
				"properties": {
					"scope": {"type": "string", "enum": ["global", "profile"]},
					"operation": {"type": "string", "enum": ["append"], "description": "Bisher nur 'append' unterstützt — content wird an den bestehenden Text angehängt."},
					"content": {"type": "string", "description": "Der anzuhängende Text."}
				},
				"required": ["scope", "operation", "content"]
			}`),
		},
		{
			Name:        "geocode",
			Description: "Löst eine Adresse in Koordinaten (Breitengrad/Längengrad) auf, z.B. wenn eine Quelle nur eine Adresse aber keine Koordinaten nennt.",
			InputSchema: json.RawMessage(`{
				"properties": {
					"address": {"type": "string", "description": "Aufzulösende Adresse."},
					"country": {"type": "string", "description": "ISO 3166-1 alpha-2 Ländercode zur Eingrenzung, z.B. SE, AT, PT. Optional."}
				},
				"required": ["address"]
			}`),
		},
		{
			Name:        "list_profile_listings",
			Description: "Liefert eine Übersicht (Titel, Preis, Score, Status, Quell-URL) aller bestehenden Inserate in diesem Profil — für Vergleichsfragen (\"vergleiche mit Kappmuren 2\") und um vor dem Anlegen eines neuen Inserats zu prüfen, ob die Quell-URL schon existiert.",
			InputSchema: json.RawMessage(`{"properties": {}, "required": []}`),
		},
		{
			Name:        "compute_ranking",
			Description: "Liefert die nach Score absteigend sortierte Rangliste aller bewerteten, nicht abgelehnten/archivierten Inserate im Profil. Rein deterministisch, kein LLM-Call — das Kurzfazit für den Chat formulierst du selbst im Antworttext, nicht dieses Tool.",
			InputSchema: json.RawMessage(`{"properties": {}, "required": []}`),
		},
	}
}

// ListingWriter is the narrow slice of repository.ListingRepository the
// Agent's tools need: Create/GetByID/Update for save_listing,
// SetTitleImageIfUnset for download_image's auto-title-image behavior
// (mirrors internal/service.ImageService.Upload, Layer 3.2), ListByProfile
// for list_profile_listings and ListRankable for compute_ranking (Layer
// 6.6) — both plain reads, kept on this interface rather than a separate
// one since it already isn't purely a "writer" (GetByID is a read too).
type ListingWriter interface {
	Create(ctx context.Context, listing *domain.Listing) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Listing, error)
	Update(ctx context.Context, listing *domain.Listing) error
	SetTitleImageIfUnset(ctx context.Context, listingID, imageID uuid.UUID) (bool, error)
	ListByProfile(ctx context.Context, profileID uuid.UUID, needsReevaluation *bool) ([]domain.Listing, error)
	ListRankable(ctx context.Context, profileID uuid.UUID) ([]domain.Listing, error)
}

// ImageWriter is the narrow slice of repository.ImageRepository
// download_image needs.
type ImageWriter interface {
	Create(ctx context.Context, image *domain.ListingImage) error
	CountByListing(ctx context.Context, listingID uuid.UUID) (int, error)
}

// PageFetcher backs fetch_url. Satisfied by *fetch.Client.
type PageFetcher interface {
	FetchPage(ctx context.Context, pageURL string) (*fetch.Result, error)
}

// ImageDownloader backs download_image. Satisfied by *fetch.Client.
type ImageDownloader interface {
	DownloadImage(ctx context.Context, imageURL string) (*fetch.DownloadedImage, error)
}

// ListingEvaluator backs evaluate_listing — the same single-shot Grade
// computation as POST /api/listings/{id}/evaluate (Layer 2.2), expressed
// with only internal/domain types so internal/agent never needs to import
// internal/service (which already imports internal/agent for
// AgentService — importing back would cycle). The service package adapts
// its *EvaluationService to this interface.
type ListingEvaluator interface {
	EvaluateListing(ctx context.Context, listingID uuid.UUID) (*domain.Listing, []domain.CriterionScore, error)
}

// AddressGeocoder backs the geocode Tool. Satisfied by an adapter over the
// package-level geocode.Geocode func (internal/geocode has no client type
// to implement this directly).
type AddressGeocoder interface {
	Geocode(ctx context.Context, address, country string) (*geocode.Result, error)
}

var validImageCategories = map[string]bool{
	"exterior":    true,
	"interior":    true,
	"floor_plan":  true,
	"outbuilding": true,
	"other":       true,
}

// Registry dispatches Tool calls for one Agent turn. It is scoped to a
// single profile — construct a fresh Registry per turn (AgentService does
// this in internal/service/agent_service.go) rather than sharing one
// across turns/sessions, since TouchedListingID accumulates per-turn state
// for the inline Listing card (chat_message.listing_id).
type Registry struct {
	ProfileID  uuid.UUID
	Listings   ListingWriter
	Images     ImageWriter
	Fetcher    PageFetcher
	Downloader ImageDownloader
	Evaluator  ListingEvaluator
	Geocoder   AddressGeocoder
	DataDir    string

	touchedListingID *uuid.UUID
}

// TouchedListingID returns the listing save_listing or download_image last
// wrote to during this turn, or nil if neither ran — used to set the final
// assistant chat_message's listing_id so the frontend can render the
// inline Listing card (spec 00-03 Layer 6.4).
func (r *Registry) TouchedListingID() *uuid.UUID {
	return r.touchedListingID
}

// Dispatch implements DispatchFunc, routing a ToolCall to its
// implementation by name.
func (r *Registry) Dispatch(ctx context.Context, call ToolCall) (string, bool) {
	switch call.Name {
	case "fetch_url":
		return r.fetchURL(ctx, call.Arguments)
	case "save_listing":
		return r.saveListing(ctx, call.Arguments)
	case "download_image":
		return r.downloadImage(ctx, call.Arguments)
	case "evaluate_listing":
		return r.evaluateListing(ctx, call.Arguments)
	case "propose_criteria":
		return r.proposeCriteria(call.Arguments)
	case "update_context":
		return r.updateContext(call.Arguments)
	case "geocode":
		return r.geocodeAddress(ctx, call.Arguments)
	case "list_profile_listings":
		return r.listProfileListings(ctx)
	case "compute_ranking":
		return r.computeRanking(ctx)
	default:
		return errorJSON("unbekanntes Tool: " + call.Name), true
	}
}

func (r *Registry) fetchURL(ctx context.Context, raw json.RawMessage) (string, bool) {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &args); err != nil || strings.TrimSpace(args.URL) == "" {
		return errorJSON("url ist erforderlich"), true
	}

	result, err := r.Fetcher.FetchPage(ctx, args.URL)
	if err != nil {
		return errorJSON(err.Error()), true
	}
	return mustJSON(map[string]any{
		"title":      result.Title,
		"text":       result.Text,
		"image_urls": result.ImageURLs,
		"links":      result.Links,
	}), false
}

// saveListingFields mirrors the save_listing schema's "fields" object —
// every key optional, absent keys leave the current value untouched.
type saveListingFields struct {
	Title          *string  `json:"title"`
	SourceURL      *string  `json:"source_url"`
	Price          *float64 `json:"price"`
	PriceCurrency  *string  `json:"price_currency"`
	PropertySizeM2 *float64 `json:"property_size_m2"`
	PlotSizeM2     *float64 `json:"plot_size_m2"`
	Address        *string  `json:"address"`
	Region         *string  `json:"region"`
	Country        *string  `json:"country"`
	Latitude       *float64 `json:"latitude"`
	Longitude      *float64 `json:"longitude"`
	Description    *string  `json:"description"`
	DocumentMD     *string  `json:"document_md"`
	SellerName     *string  `json:"seller_name"`
	SellerPhone    *string  `json:"seller_phone"`
	SellerEmail    *string  `json:"seller_email"`
	SellerNotes    *string  `json:"seller_notes"`
}

func (r *Registry) saveListing(ctx context.Context, raw json.RawMessage) (string, bool) {
	var args struct {
		ListingID      *uuid.UUID        `json:"listing_id"`
		Fields         saveListingFields `json:"fields"`
		ForceOverwrite []string          `json:"force_overwrite"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return errorJSON("ungültige Argumente: " + err.Error()), true
	}

	now := time.Now().UTC()
	creating := args.ListingID == nil
	var listing *domain.Listing
	if creating {
		listing = &domain.Listing{
			ID:                  uuid.New(),
			ProfileID:           r.ProfileID,
			Status:              domain.ListingStatusNew,
			SourceType:          domain.SourceTypeURL,
			UserConfirmedFields: []string{},
			CreatedAt:           now,
		}
	} else {
		existing, err := r.Listings.GetByID(ctx, *args.ListingID)
		if err != nil {
			return errorJSON(err.Error()), true
		}
		if existing.ProfileID != r.ProfileID {
			return errorJSON("Inserat gehört nicht zu diesem Profil"), true
		}
		listing = existing
	}

	skipped := applyListingFields(listing, args.Fields, args.ForceOverwrite)
	listing.UpdatedAt = now

	var err error
	if creating {
		err = r.Listings.Create(ctx, listing)
	} else {
		err = r.Listings.Update(ctx, listing)
	}
	if err != nil {
		return errorJSON(err.Error()), true
	}

	r.touchedListingID = &listing.ID

	response := map[string]any{"status": "ok", "listing_id": listing.ID.String()}
	if len(skipped) > 0 {
		response["skipped_fields"] = skipped
		response["note"] = "Manuell bestätigte Felder wurden nicht überschrieben: " + strings.Join(skipped, ", ")
	}
	return mustJSON(response), false
}

// applyListingFields merges fields onto listing, skipping any field that's
// both in listing.UserConfirmedFields and absent from forceOverwrite (ADR
// 0016). A field written via forceOverwrite is removed from
// UserConfirmedFields — the Agent's new value replaces the user's
// correction. document_md is prose, not a structured field, so it's never
// protected (mirrors internal/service.ListingService.applyPatch's manual
// -edit path, which never adds document_md to UserConfirmedFields either).
// Returns the names of fields that were skipped.
func applyListingFields(listing *domain.Listing, fields saveListingFields, forceOverwrite []string) []string {
	type write struct {
		field string
		apply func()
	}
	var writes []write
	add := func(field string, present bool, apply func()) {
		if present {
			writes = append(writes, write{field, apply})
		}
	}

	add("title", fields.Title != nil, func() { listing.Title = *fields.Title })
	add("source_url", fields.SourceURL != nil, func() { listing.SourceURL = fields.SourceURL })
	add("price", fields.Price != nil, func() { listing.Price = fields.Price })
	add("price_currency", fields.PriceCurrency != nil, func() { listing.PriceCurrency = fields.PriceCurrency })
	add("property_size_m2", fields.PropertySizeM2 != nil, func() { listing.PropertySizeM2 = fields.PropertySizeM2 })
	add("plot_size_m2", fields.PlotSizeM2 != nil, func() { listing.PlotSizeM2 = fields.PlotSizeM2 })
	add("address", fields.Address != nil, func() { listing.Address = fields.Address })
	add("region", fields.Region != nil, func() { listing.Region = fields.Region })
	add("country", fields.Country != nil, func() { listing.Country = fields.Country })
	add("latitude", fields.Latitude != nil, func() { listing.Latitude = fields.Latitude })
	add("longitude", fields.Longitude != nil, func() { listing.Longitude = fields.Longitude })
	add("description", fields.Description != nil, func() { listing.Description = fields.Description })
	add("seller_name", fields.SellerName != nil, func() { listing.SellerName = fields.SellerName })
	add("seller_phone", fields.SellerPhone != nil, func() { listing.SellerPhone = fields.SellerPhone })
	add("seller_email", fields.SellerEmail != nil, func() { listing.SellerEmail = fields.SellerEmail })
	add("seller_notes", fields.SellerNotes != nil, func() { listing.SellerNotes = fields.SellerNotes })

	var skipped []string
	for _, w := range writes {
		if slices.Contains(listing.UserConfirmedFields, w.field) && !slices.Contains(forceOverwrite, w.field) {
			skipped = append(skipped, w.field)
			continue
		}
		w.apply()
		if slices.Contains(forceOverwrite, w.field) {
			listing.UserConfirmedFields = removeString(listing.UserConfirmedFields, w.field)
		}
	}

	if fields.DocumentMD != nil {
		listing.DocumentMD = fields.DocumentMD
	}
	return skipped
}

func removeString(values []string, target string) []string {
	result := values[:0]
	for _, v := range values {
		if v != target {
			result = append(result, v)
		}
	}
	return result
}

func (r *Registry) downloadImage(ctx context.Context, raw json.RawMessage) (string, bool) {
	var args struct {
		ListingID uuid.UUID `json:"listing_id"`
		ImageURL  string    `json:"image_url"`
		Category  *string   `json:"category"`
	}
	if err := json.Unmarshal(raw, &args); err != nil || args.ListingID == uuid.Nil || strings.TrimSpace(args.ImageURL) == "" {
		return errorJSON("listing_id und image_url sind erforderlich"), true
	}
	if args.Category != nil && !validImageCategories[*args.Category] {
		return errorJSON("unbekannte Kategorie: " + *args.Category), true
	}

	listing, err := r.Listings.GetByID(ctx, args.ListingID)
	if err != nil {
		return errorJSON(err.Error()), true
	}
	if listing.ProfileID != r.ProfileID {
		return errorJSON("Inserat gehört nicht zu diesem Profil"), true
	}

	downloaded, err := r.Downloader.DownloadImage(ctx, args.ImageURL)
	if err != nil {
		return errorJSON(err.Error()), true
	}

	count, err := r.Images.CountByListing(ctx, args.ListingID)
	if err != nil {
		return errorJSON(err.Error()), true
	}

	dir := filepath.Join(r.DataDir, "images", "listings", args.ListingID.String())
	if mkdirErr := os.MkdirAll(dir, 0o750); mkdirErr != nil {
		return errorJSON("Bildverzeichnis konnte nicht angelegt werden: " + mkdirErr.Error()), true
	}

	imageID := uuid.New()
	relPath := filepath.Join("images", "listings", args.ListingID.String(), imageID.String()+downloaded.Extension)
	absPath := filepath.Join(r.DataDir, relPath)
	if writeErr := os.WriteFile(absPath, downloaded.Data, 0o600); writeErr != nil {
		return errorJSON("Bild konnte nicht geschrieben werden: " + writeErr.Error()), true
	}

	image := &domain.ListingImage{
		ID:        imageID,
		ListingID: args.ListingID,
		URL:       args.ImageURL,
		IsLocal:   true,
		SortOrder: count,
		FilePath:  filepath.ToSlash(relPath),
		Category:  args.Category,
		CreatedAt: time.Now().UTC(),
	}
	if createErr := r.Images.Create(ctx, image); createErr != nil {
		_ = os.Remove(absPath)
		return errorJSON(createErr.Error()), true
	}
	if _, titleErr := r.Listings.SetTitleImageIfUnset(ctx, args.ListingID, imageID); titleErr != nil {
		return errorJSON(titleErr.Error()), true
	}

	r.touchedListingID = &args.ListingID
	return mustJSON(map[string]any{
		"status":   "ok",
		"image_id": imageID.String(),
		"path":     "/media/" + filepath.ToSlash(relPath),
	}), false
}

func (r *Registry) evaluateListing(ctx context.Context, raw json.RawMessage) (string, bool) {
	var args struct {
		ListingID uuid.UUID `json:"listing_id"`
	}
	if err := json.Unmarshal(raw, &args); err != nil || args.ListingID == uuid.Nil {
		return errorJSON("listing_id ist erforderlich"), true
	}

	listing, err := r.Listings.GetByID(ctx, args.ListingID)
	if err != nil {
		return errorJSON(err.Error()), true
	}
	if listing.ProfileID != r.ProfileID {
		return errorJSON("Inserat gehört nicht zu diesem Profil"), true
	}

	updated, scores, err := r.Evaluator.EvaluateListing(ctx, args.ListingID)
	if err != nil {
		return errorJSON(err.Error()), true
	}

	r.touchedListingID = &updated.ID
	grades := make([]map[string]any, len(scores))
	for i, score := range scores {
		grades[i] = map[string]any{
			"criterion_id": score.CriterionID,
			"grade":        score.Grade,
			"reasoning":    score.Reasoning,
		}
	}
	return mustJSON(map[string]any{
		"status":        "ok",
		"listing_id":    updated.ID.String(),
		"overall_score": updated.OverallScore,
		"grades":        grades,
	}), false
}

// proposeCriteria validates the shape of a Criterion proposal and returns
// pending_confirmation without writing anything (ADR 0009/0004) — the
// proposal itself lives in the tool call's arguments, already persisted
// verbatim in chat_message.tool_calls by the caller (AgentService), and is
// only applied once the user confirms via
// POST /api/chat/messages/{id}/tool-calls/{idx}/confirm.
func (r *Registry) proposeCriteria(raw json.RawMessage) (string, bool) {
	var args struct {
		Action       string  `json:"action"`
		CriterionID  *string `json:"criterion_id"`
		Name         string  `json:"name"`
		Description  string  `json:"description"`
		IsSubjective bool    `json:"is_subjective"`
		Weight       *int    `json:"weight"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return errorJSON("ungültige Argumente: " + err.Error()), true
	}
	if args.Action != "add" && args.Action != "edit" {
		return errorJSON("action muss 'add' oder 'edit' sein"), true
	}
	if strings.TrimSpace(args.Name) == "" {
		return errorJSON("name ist erforderlich"), true
	}
	if args.Action == "edit" && (args.CriterionID == nil || strings.TrimSpace(*args.CriterionID) == "") {
		return errorJSON("criterion_id ist erforderlich bei action=edit"), true
	}
	if args.Weight != nil && (*args.Weight < 1 || *args.Weight > 5) {
		return errorJSON("weight muss zwischen 1 und 5 liegen"), true
	}

	verb := "Vorgeschlagen"
	if args.Action == "edit" {
		verb = "Änderung vorgeschlagen"
	}
	return mustJSON(map[string]any{
		"status":  "pending_confirmation",
		"summary": fmt.Sprintf("%s: Kriterium %q — ⚠️ Noch nicht aktiv, Bestätigung ausstehend.", verb, args.Name),
	}), false
}

// updateContext validates the shape of a Context proposal and returns
// pending_confirmation without writing anything — same pattern as
// proposeCriteria.
func (r *Registry) updateContext(raw json.RawMessage) (string, bool) {
	var args struct {
		Scope     string `json:"scope"`
		Operation string `json:"operation"`
		Content   string `json:"content"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return errorJSON("ungültige Argumente: " + err.Error()), true
	}
	if args.Scope != "global" && args.Scope != "profile" {
		return errorJSON("scope muss 'global' oder 'profile' sein"), true
	}
	if args.Operation != "append" {
		return errorJSON("operation muss 'append' sein"), true
	}
	if strings.TrimSpace(args.Content) == "" {
		return errorJSON("content darf nicht leer sein"), true
	}

	return mustJSON(map[string]any{
		"status":  "pending_confirmation",
		"summary": fmt.Sprintf("Kontext-Ergänzung vorgeschlagen (%s) — ⚠️ Noch nicht aktiv, Bestätigung ausstehend.", args.Scope),
	}), false
}

func (r *Registry) geocodeAddress(ctx context.Context, raw json.RawMessage) (string, bool) {
	var args struct {
		Address string `json:"address"`
		Country string `json:"country"`
	}
	if err := json.Unmarshal(raw, &args); err != nil || strings.TrimSpace(args.Address) == "" {
		return errorJSON("address ist erforderlich"), true
	}

	result, err := r.Geocoder.Geocode(ctx, args.Address, args.Country)
	if err != nil {
		if errors.Is(err, geocode.ErrNotFound) {
			return errorJSON("Adresse nicht gefunden: " + args.Address), true
		}
		return errorJSON(err.Error()), true
	}
	return mustJSON(map[string]any{
		"latitude":     result.Lat,
		"longitude":    result.Lon,
		"display_name": result.DisplayName,
	}), false
}

func (r *Registry) listProfileListings(ctx context.Context) (string, bool) {
	listings, err := r.Listings.ListByProfile(ctx, r.ProfileID, nil)
	if err != nil {
		return errorJSON(err.Error()), true
	}

	summaries := make([]map[string]any, len(listings))
	for i, listing := range listings {
		summaries[i] = map[string]any{
			"listing_id":     listing.ID.String(),
			"title":          listing.Title,
			"price":          listing.Price,
			"price_currency": listing.PriceCurrency,
			"score":          listing.OverallScore,
			"status":         listing.Status,
			"source_url":     listing.SourceURL,
		}
	}
	return mustJSON(map[string]any{"listings": summaries}), false
}

func (r *Registry) computeRanking(ctx context.Context) (string, bool) {
	listings, err := r.Listings.ListRankable(ctx, r.ProfileID)
	if err != nil {
		return errorJSON(err.Error()), true
	}

	ranking := make([]map[string]any, len(listings))
	for i, listing := range listings {
		ranking[i] = map[string]any{
			"rank":       i + 1,
			"listing_id": listing.ID.String(),
			"title":      listing.Title,
			"price":      listing.Price,
			"score":      listing.OverallScore,
		}
	}
	return mustJSON(map[string]any{"ranking": ranking}), false
}

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(data)
}

func errorJSON(message string) string {
	return mustJSON(map[string]string{"error": message})
}
