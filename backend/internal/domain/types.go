// Package domain holds the types shared across service and repository code:
// aggregates, value objects, and the error types the API layer maps to HTTP
// status codes. Types are added incrementally as each implementation layer
// needs them, not ported wholesale from dev-property-finder up front.
package domain

import (
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
	ID           uuid.UUID `json:"id"`
	ProfileID    uuid.UUID `json:"profile_id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Weight       int       `json:"weight"`
	IsSubjective bool      `json:"is_subjective"`
	SortOrder    int       `json:"sort_order"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

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

// TaskType enumerates the kinds of background tasks the app can run. Only
// bulk re-evaluation exists so far (spec 01-04 Layer 2.3, ADR 0015); chat
// turns are fire-and-forget instead (ADR 0008) and don't use this table.
type TaskType string

const TaskTypeListingsReevaluateAll TaskType = "listings.reevaluate_all"

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

// UpstreamError signals 502 Bad Gateway — a downstream service (the LLM
// provider) failed transiently. Distinct from ValidationError: the request
// itself was well-formed, retrying later may succeed.
type UpstreamError struct {
	Message string `json:"message"`
}

func (err *UpstreamError) Error() string { return err.Message }
