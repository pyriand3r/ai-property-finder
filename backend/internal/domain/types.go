// Package domain holds the types shared across service and repository code:
// aggregates, value objects, and the error types the API layer maps to HTTP
// status codes. Types are added incrementally as each implementation layer
// needs them, not ported wholesale from dev-property-finder up front.
package domain

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/google/uuid"
)

// SearchProfile is a named, user-defined set of Criteria describing the
// desired property (spec 01-08). Unlike dev-property-finder, Description is
// optional free text for the user's own reference — it is never parsed.
type SearchProfile struct {
	ID             uuid.UUID  `json:"id"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	ArchivedAt     *time.Time `json:"archived_at,omitempty"`
	RankingSummary *string    `json:"ranking_summary,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Criterion is a single evaluable dimension of a Listing (spec 01-04).
// IsSubjective mirrors the criterion.is_subjective DB column directly —
// deliberately not inverted to an "is_objective" wire field, to avoid a
// double-negative source of bugs between API and storage.
type Criterion struct {
	ID           uuid.UUID       `json:"id"`
	ProfileID    uuid.UUID       `json:"profile_id"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Weight       int             `json:"weight"`
	IsSubjective bool            `json:"is_subjective"`
	SortOrder    int             `json:"sort_order"`
	Status       CriterionStatus `json:"status"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// CriterionStatus is the lifecycle state of a Criterion (docs/adr/0024).
// Only active criteria are used by evaluation and scoring; a proposed one
// is visible and editable but ignored until the user approves it.
type CriterionStatus string

const (
	CriterionStatusProposed CriterionStatus = "proposed"
	CriterionStatusActive   CriterionStatus = "active"
)

// CriterionGrade is the A-F letter mark the AI assigns to one Criterion for
// one Listing (carried over from dev-property-finder, unchanged — spec
// 01-04). A = best fit, F = worst fit.
type CriterionGrade string

const (
	GradeA CriterionGrade = "A"
	GradeB CriterionGrade = "B"
	GradeC CriterionGrade = "C"
	GradeD CriterionGrade = "D"
	GradeE CriterionGrade = "E"
	GradeF CriterionGrade = "F"
)

// CriterionScore is the per-Criterion result returned by the AI provider:
// an ordered (by position) Grade plus reasoning (spec 01-04, ADR 0006 —
// mapped by position, never by an ID the model echoes back). Index is
// backend-only bookkeeping, not part of the wire response.
type CriterionScore struct {
	Index       int            `json:"-"`
	CriterionID uuid.UUID      `json:"criterion_id"`
	Grade       CriterionGrade `json:"grade"`
	Reasoning   string         `json:"reasoning"`
}

// ListingGrade is one row of the grade grid on the Listing Detail View
// (spec 01-07 Layer 5.3) — a persisted Grade joined with its Criterion's
// name for display. Unlike CriterionScore (the AI provider's transient
// output shape), this is a read model over already-stored data.
type ListingGrade struct {
	CriterionID   uuid.UUID      `json:"criterion_id"`
	CriterionName string         `json:"criterion_name"`
	Grade         CriterionGrade `json:"grade"`
	Reasoning     string         `json:"reasoning"`
}

// ListingStatus is the lifecycle state of a Listing (carried over from
// dev-property-finder, unchanged — spec 01-07).
type ListingStatus string

const (
	ListingStatusNew           ListingStatus = "new"
	ListingStatusWaiting       ListingStatus = "waiting"
	ListingStatusInNegotiation ListingStatus = "in_negotiation"
	ListingStatusVisited       ListingStatus = "visited"
	ListingStatusRejected      ListingStatus = "rejected"
	ListingStatusArchived      ListingStatus = "archived"
)

// SourceType records how a Listing entered the system (spec 00-02). Layer 2
// only produces "manual"; the other values are written once chat ingestion
// (Layer 6+) lands.
type SourceType string

const (
	SourceTypeURL    SourceType = "url"
	SourceTypePDF    SourceType = "pdf"
	SourceTypeImage  SourceType = "image"
	SourceTypeManual SourceType = "manual"
)

// Listing is a real-estate offering (spec 01-07, 00-02). LastContactAt
// ("Letztkontakt") is orthogonal to Status — it tracks seller outreach
// only. OverallScore is nil until the first Evaluation completes.
// UserConfirmedFields lists structured field names the user has manually
// edited since the last Agent write (ADR 0016) — the Agent asks before
// overwriting them; document_md/kurzfazit are prose, not structured
// fields, and are never listed here.
type Listing struct {
	ID                  uuid.UUID     `json:"id"`
	ProfileID           uuid.UUID     `json:"profile_id"`
	Title               string        `json:"title"`
	SourceURL           *string       `json:"source_url,omitempty"`
	Price               *float64      `json:"price,omitempty"`
	PriceCurrency       *string       `json:"price_currency,omitempty"`
	PropertySizeM2      *float64      `json:"property_size_m2,omitempty"`
	PlotSizeM2          *float64      `json:"plot_size_m2,omitempty"`
	Address             *string       `json:"address,omitempty"`
	Region              *string       `json:"region,omitempty"`
	Country             *string       `json:"country,omitempty"`
	Latitude            *float64      `json:"latitude,omitempty"`
	Longitude           *float64      `json:"longitude,omitempty"`
	Description         *string       `json:"description,omitempty"`
	TitleImagePath      *string       `json:"title_image_path,omitempty"`
	TitleImageID        *uuid.UUID    `json:"title_image_id,omitempty"`
	Status              ListingStatus `json:"status"`
	RejectionComment    *string       `json:"rejection_comment,omitempty"`
	OverallScore        *float64      `json:"overall_score,omitempty"`
	SellerName          *string       `json:"seller_name,omitempty"`
	SellerPhone         *string       `json:"seller_phone,omitempty"`
	SellerEmail         *string       `json:"seller_email,omitempty"`
	SellerNotes         *string       `json:"seller_notes,omitempty"`
	LastContactAt       *time.Time    `json:"last_contact_at,omitempty"`
	DocumentMD          *string       `json:"document_md,omitempty"`
	SourceType          SourceType    `json:"source_type"`
	SourceFilePath      *string       `json:"source_file_path,omitempty"`
	UserConfirmedFields []string      `json:"user_confirmed_fields"`
	NeedsReevaluation   bool          `json:"needs_reevaluation"`
	Kurzfazit           *string       `json:"kurzfazit,omitempty"`
	CreatedAt           time.Time     `json:"created_at"`
	UpdatedAt           time.Time     `json:"updated_at"`
}

// ListingImage is one photo attached to a Listing (ADR 0012). URL is the
// remote source the Agent downloaded it from; a directly user-uploaded
// image (Layer 3.1, plain-form fallback per ADR 0001) has no remote
// source, so URL is "" and IsLocal is true from the start. FilePath is
// relative to DATA_DIR: "images/listings/{listing_id}/{uuid}.{ext}",
// served at /media/{file_path}.
type ListingImage struct {
	ID        uuid.UUID `json:"id"`
	ListingID uuid.UUID `json:"listing_id"`
	URL       string    `json:"url"`
	IsLocal   bool      `json:"is_local"`
	SortOrder int       `json:"sort_order"`
	FilePath  string    `json:"file_path"`
	Category  *string   `json:"category,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ListingNote is a freeform, timestamped note a user attaches to a Listing
// (spec 01-11, RF.4) — the user's own running notes about the property
// ("called the agent, viewing booked for Saturday"). Distinct from
// Listing.SellerNotes, which is a single field about the seller/contact.
type ListingNote struct {
	ID        uuid.UUID `json:"id"`
	ListingID uuid.UUID `json:"listing_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TaskType enumerates the kinds of background tasks the app can run: bulk
// re-evaluation (spec 01-04 Layer 2.3, ADR 0015) and portal search (spec
// 01-16, ADR 0028, the only type that is resumed after a restart). Chat
// turns are fire-and-forget instead (ADR 0008) and don't use this table.
type TaskType string

const (
	TaskTypeListingsReevaluateAll TaskType = "listings.reevaluate_all"
	TaskTypePortalSearch          TaskType = "portal.search"
)

// TaskStatus is the lifecycle state of a Task row. There is no "queued"
// state here — a Task starts "running" the moment its goroutine is
// spawned; nothing is dequeued later (spec 01-04: "sequenzielle
// Goroutine", not a polling worker).
type TaskStatus string

const (
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusSucceeded TaskStatus = "succeeded"
	TaskStatusFailed    TaskStatus = "failed"
)

// Task is the durable record of a background task, surfaced to the
// frontend via SSE (internal/events) and readable directly for reconnect
// reconciliation.
type Task struct {
	ID          uuid.UUID  `json:"id"`
	Type        TaskType   `json:"type"`
	Status      TaskStatus `json:"status"`
	ProfileID   *uuid.UUID `json:"profile_id,omitempty"`
	ListingID   *uuid.UUID `json:"listing_id,omitempty"`
	Total       *int       `json:"total,omitempty"`
	Done        int        `json:"done"`
	FailedCount int        `json:"failed_count"`
	Error       *string    `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// ContextScope distinguishes the single Global Context from a per-profile
// Profile Context (spec 01-02).
type ContextScope string

const (
	ContextScopeGlobal  ContextScope = "global"
	ContextScopeProfile ContextScope = "profile"
)

// Context is a Markdown text store that steers the Agent — see
// docs/CONTEXT.md "Context File". content_md is never nil; an unwritten
// Context is an empty string, not a missing row (both scopes are seeded on
// creation).
type Context struct {
	ID        uuid.UUID    `json:"id"`
	Scope     ContextScope `json:"scope"`
	ProfileID *uuid.UUID   `json:"profile_id,omitempty"`
	ContentMD string       `json:"content_md"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// ChangedBy identifies who produced a ContextRevision.
type ChangedBy string

const (
	ChangedByUser  ChangedBy = "user"
	ChangedByAgent ChangedBy = "agent"
)

// ContextRevision is an append-only audit row written only when
// content_md actually changes. ContentMD is the full snapshot from BEFORE
// the change, not a diff (spec 00-02).
type ContextRevision struct {
	ID            uuid.UUID  `json:"id"`
	ContextID     uuid.UUID  `json:"context_id"`
	ContentMD     string     `json:"content_md"`
	ChangedBy     ChangedBy  `json:"changed_by"`
	ChatMessageID *uuid.UUID `json:"chat_message_id,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// ChatSession is one conversation thread for a Search Profile. A profile
// owns exactly one *active* session (ended_at IS NULL) at a time; starting
// a new one ends the previous — the old session becomes read-only history
// (spec 01-01, Layer 4.1). The first session is created alongside the
// profile itself (Layer 1.1).
type ChatSession struct {
	ID                 uuid.UUID  `json:"id"`
	ProfileID          uuid.UUID  `json:"profile_id"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	EndedAt            *time.Time `json:"ended_at,omitempty"`
	TokenCountEstimate *int       `json:"token_count_estimate,omitempty"`
}

// ChatRole distinguishes who/what produced a ChatMessage.
type ChatRole string

const (
	ChatRoleUser      ChatRole = "user"
	ChatRoleAssistant ChatRole = "assistant"
	ChatRoleTool      ChatRole = "tool"
)

// ChatMessage is one turn in a ChatSession. Layer 4.1 only ever writes
// role=user rows (no Agent yet, spec 01-04 Layer 4.1: "nur
// User-Nachrichten, noch kein Agent") — ToolCalls/ToolResult/ToolName
// exist on the type now because the column does, but stay unset until the
// Agent loop lands (Layer 6).
type ChatMessage struct {
	ID             uuid.UUID       `json:"id"`
	SessionID      uuid.UUID       `json:"session_id"`
	Role           ChatRole        `json:"role"`
	Content        string          `json:"content"`
	ToolCalls      json.RawMessage `json:"tool_calls,omitempty"`
	ToolResult     json.RawMessage `json:"tool_result,omitempty"`
	ToolName       *string         `json:"tool_name,omitempty"`
	AttachmentPath *string         `json:"attachment_path,omitempty"`
	ListingID      *uuid.UUID      `json:"listing_id,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

// ── Hazard ratings ──────────────────────────────────────────────────────

// HazardType is a natural-hazard category a region is rated for
// (docs/adr/0025). The set is closed: the hazard_source CHECK constraint
// lists the same four values.
type HazardType string

const (
	HazardWildfire   HazardType = "wildfire"
	HazardFlood      HazardType = "flood"
	HazardEarthquake HazardType = "earthquake"
	HazardLandslide  HazardType = "landslide"
)

// HazardTypes returns every HazardType in display order — the order the API
// and the docs use.
func HazardTypes() []HazardType {
	return []HazardType{HazardWildfire, HazardFlood, HazardEarthquake, HazardLandslide}
}

// Valid reports whether t is one of the four known hazard types.
func (t HazardType) Valid() bool { return slices.Contains(HazardTypes(), t) }

// A hazard rating runs from HazardRatingMin (very low hazard) to
// HazardRatingMax (very high hazard). Higher is worse — the opposite
// direction of a Listing's Score, which the rating never feeds into.
const (
	HazardRatingMin = 1
	HazardRatingMax = 5
)

// HazardRegion is a geographic unit hazards are rated for. Key is
// "<ISO 3166-1 alpha-2 country>-<national code>"; for Portugal it is
// "PT-<4-digit DICO code of the municipality>" (for example PT-1106 for
// Lisboa). ParentName (district or island) tells apart regions that share a
// Name. NameSearch is Name lower-cased and without diacritics, filled when a
// dataset is loaded; it is only used for lookups and never sent to clients.
type HazardRegion struct {
	Key        string `json:"key"`
	Country    string `json:"country"`
	Name       string `json:"name"`
	ParentName string `json:"parent_name"`
	NameSearch string `json:"-"`
}

// HazardSource records which published dataset a rating was derived from.
// Method says how the dataset's own classes became the 1-5 scale and whether
// that step is an app-specific interpretation. RetrievedOn is a YYYY-MM-DD
// date.
type HazardSource struct {
	ID             string     `json:"id"`
	HazardType     HazardType `json:"hazard_type"`
	Title          string     `json:"title"`
	Publisher      string     `json:"publisher"`
	URL            string     `json:"url"`
	License        string     `json:"license"`
	DatasetVersion string     `json:"dataset_version"`
	RetrievedOn    string     `json:"retrieved_on"`
	Method         string     `json:"method"`
	Notes          string     `json:"notes"`
}

// HazardRating is one (region, hazard type) rating as a dataset file
// states it. The stored row also carries updated_at, which the database
// sets; clients get it through HazardRatingDetail.
type HazardRating struct {
	RegionKey  string     `json:"region_key"`
	HazardType HazardType `json:"hazard_type"`
	Rating     int        `json:"rating"`
	SourceID   string     `json:"source_id"`
}

// HazardRatingDetail is a stored rating joined with its source, the shape
// the API returns. UpdatedAt is when the rating value or its source_id last
// changed in the database; syncing identical data does not move it.
type HazardRatingDetail struct {
	HazardType HazardType   `json:"hazard_type"`
	Rating     int          `json:"rating"`
	UpdatedAt  time.Time    `json:"updated_at"`
	Source     HazardSource `json:"source"`
}

// RegionHazards is a region together with all its ratings, in HazardTypes
// order. A hazard without usable source data for the region is absent from
// Ratings.
type RegionHazards struct {
	HazardRegion
	Ratings []HazardRatingDetail `json:"ratings"`
}

// HazardCatalogStatus says whether the app ships a catalog entry's data.
type HazardCatalogStatus string

const (
	// HazardCatalogBundled: the data is in an embedded dataset file and
	// SourceID names its hazard_source row.
	HazardCatalogBundled HazardCatalogStatus = "bundled"
	// HazardCatalogCandidate: a known public source that is not ingested yet.
	HazardCatalogCandidate HazardCatalogStatus = "candidate"
)

// HazardCatalogEntry is one row of the source catalog (docs/adr/0027,
// spec 01-15): a hazard data source known for a country, shipped or not.
// Scope says which part of the country it covers ("" = all of it). For a
// bundled entry Title, Publisher, URL and License are copied from the
// dataset source SourceID, so they are stated once.
type HazardCatalogEntry struct {
	ID         string              `json:"id"`
	Country    string              `json:"country"`
	HazardType HazardType          `json:"hazard_type"`
	Scope      string              `json:"scope"`
	Status     HazardCatalogStatus `json:"status"`
	SourceID   string              `json:"source_id,omitempty"`
	Title      string              `json:"title"`
	Publisher  string              `json:"publisher"`
	URL        string              `json:"url"`
	License    string              `json:"license"`
	Notes      string              `json:"notes"`
}

// HazardSuggestionStatus is the review state of a HazardSourceSuggestion.
type HazardSuggestionStatus string

const (
	// HazardSuggestionProposed: made by the Agent (or an MCP client), not yet
	// reviewed by the user.
	HazardSuggestionProposed HazardSuggestionStatus = "proposed"
	// HazardSuggestionApproved: the user cleared the source for ingestion.
	HazardSuggestionApproved HazardSuggestionStatus = "approved"
	// HazardSuggestionRejected: the user does not want the source. It stays,
	// so the same source is not suggested again.
	HazardSuggestionRejected HazardSuggestionStatus = "rejected"
)

// HazardSourceSuggestion is a hazard data source the Agent found that the
// catalog does not hold (docs/adr/0027). It changes no rating: ingestion
// is a release step that needs the user's approval first.
type HazardSourceSuggestion struct {
	ID         uuid.UUID              `json:"id"`
	Country    string                 `json:"country"`
	HazardType HazardType             `json:"hazard_type"`
	Scope      string                 `json:"scope"`
	Title      string                 `json:"title"`
	Publisher  string                 `json:"publisher"`
	URL        string                 `json:"url"`
	License    string                 `json:"license"`
	Rationale  string                 `json:"rationale"`
	Status     HazardSuggestionStatus `json:"status"`
	CreatedAt  time.Time              `json:"created_at"`
	DecidedAt  *time.Time             `json:"decided_at,omitempty"`
}

// HazardSourceSuggestionInput is what a caller supplies to suggest a source.
type HazardSourceSuggestionInput struct {
	Country    string
	HazardType HazardType
	Scope      string
	Title      string
	Publisher  string
	URL        string
	License    string
	Rationale  string
}

// ── Domain Errors ───────────────────────────────────────────────────────

// ValidationError signals 400 Bad Request — invalid input.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (err *ValidationError) Error() string { return err.Message }

// NotFoundError signals 404 Not Found — an entity by ID does not exist.
type NotFoundError struct {
	Entity string `json:"entity"`
	ID     string `json:"id"`
}

func (err *NotFoundError) Error() string {
	return err.Entity + " not found: " + err.ID
}

// ConflictError signals 409 Conflict — e.g. a duplicate profile name.
type ConflictError struct {
	Message string `json:"message"`
}

func (err *ConflictError) Error() string { return err.Message }

// DuplicateMatch names why an existing listing counts as a duplicate.
type DuplicateMatch string

const (
	DuplicateMatchAddress     DuplicateMatch = "address"
	DuplicateMatchCoordinates DuplicateMatch = "coordinates"
)

// DuplicateListing is an existing listing in the same profile that has the
// same address or coordinates as a listing being created.
type DuplicateListing struct {
	ID      uuid.UUID        `json:"id"`
	Title   string           `json:"title"`
	Address *string          `json:"address,omitempty"`
	Matches []DuplicateMatch `json:"matches"`
}

// DuplicateListingError signals 409 Conflict with a list of possible
// duplicates. It is a warning, not a hard block: the caller may repeat the
// request with confirm_duplicate=true (issue #10).
type DuplicateListingError struct {
	Duplicates []DuplicateListing `json:"duplicates"`
}

func (err *DuplicateListingError) Error() string {
	return "Ein Inserat mit gleicher Adresse oder gleichen Koordinaten existiert bereits in diesem Profil"
}

// UpstreamError signals 502 Bad Gateway — a downstream service (the LLM
// provider) failed transiently. Distinct from ValidationError: the request
// itself was well-formed, retrying later may succeed.
type UpstreamError struct {
	Message string `json:"message"`
}

func (err *UpstreamError) Error() string { return err.Message }

// PortalSearchRequest is the input of the search_portal Tool (spec 01-16).
// Page 1 is URL as given; page N > 1 is URL with the query parameter
// PageParam set to N, so MaxPages > 1 needs a PageParam.
type PortalSearchRequest struct {
	URL          string
	PageParam    string
	MaxPages     int
	LinkContains string
}

// PortalSearch is the state of one portal search as the Agent sees it: the
// task row plus the links collected so far. NewLinks are links that are no
// Listing source_url of the profile yet; KnownLinks already are.
type PortalSearch struct {
	TaskID     uuid.UUID  `json:"task_id"`
	Status     TaskStatus `json:"status"`
	URL        string     `json:"url"`
	PagesDone  int        `json:"pages_done"`
	MaxPages   int        `json:"max_pages"`
	NewLinks   []string   `json:"new_links"`
	KnownLinks []string   `json:"known_links"`
	StopReason string     `json:"stop_reason,omitempty"`
	Error      *string    `json:"error,omitempty"`
}
