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
	ArchivedAt     *time.Time `json:"archivedAt,omitempty"`
	RankingSummary *string    `json:"rankingSummary,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

// Criterion is a single evaluable dimension of a Listing (spec 01-04).
// IsSubjective mirrors the criterion.is_subjective DB column directly —
// deliberately not inverted to an "isObjective" wire field, to avoid a
// double-negative source of bugs between API and storage.
type Criterion struct {
	ID           uuid.UUID `json:"id"`
	ProfileID    uuid.UUID `json:"profileId"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Weight       int       `json:"weight"`
	IsSubjective bool      `json:"isSubjective"`
	SortOrder    int       `json:"sortOrder"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
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
	ProfileID *uuid.UUID   `json:"profileId,omitempty"`
	ContentMD string       `json:"contentMd"`
	CreatedAt time.Time    `json:"createdAt"`
	UpdatedAt time.Time    `json:"updatedAt"`
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
	ContextID     uuid.UUID  `json:"contextId"`
	ContentMD     string     `json:"contentMd"`
	ChangedBy     ChangedBy  `json:"changedBy"`
	ChatMessageID *uuid.UUID `json:"chatMessageId,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
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
