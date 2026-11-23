// Package config resolves both tiers of application settings: Tier 1
// (environment-only, read on every startup, never stored) and Tier 2
// (DB-backed app_config, seeded from env on first boot, DB-authoritative
// afterwards — same bootstrap shape as dev-property-finder).
package config

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Env holds settings read from the environment on every startup and never
// stored in the database. Missing required values cause LoadEnv to fail
// fast.
type Env struct {
	// DatabaseURL is the PostgreSQL connection string (always env, never DB).
	DatabaseURL string
	// HTTPListenAddr is the address the HTTP server binds to (default ":8080").
	HTTPListenAddr string
	// ClaudeAPIKey seeds app_config's llm.api_key on first boot (see
	// SeedFromEnv). Empty is valid — the LLM subsystem stays unconfigured
	// until a later layer wires an implementation to it.
	ClaudeAPIKey string
	// AppPassword gates Frontend + API access (see docs/adr/0018). Its
	// SHA-256 digest doubles as the session JWT's HMAC signing key — no
	// separate signing secret is configured.
	AppPassword string
	// CookieSecure controls the session cookie's Secure flag (docs/adr/0018).
	// Defaults to true — the documented deployment (both the root
	// docker-compose.yml used for local dev/testing and deploy/docker-
	// compose.yml used in production) sits behind an external
	// TLS-terminating reverse proxy; the container itself only ever
	// speaks plain HTTP. Set COOKIE_SECURE=false for local development
	// without a proxy in front (npm run dev's Vite proxy talks plain
	// HTTP directly to the backend) — otherwise the browser discards the
	// cookie and login loops back to /login.
	CookieSecure bool
	// DataDir is the root of the Docker volume holding uploaded/downloaded
	// files (default "/data", matching docker-compose.yml's `./data:/data`
	// mount and ADR 0012). Listing image cleanup on delete removes
	// {DataDir}/images/listings/{listing_id}. Configurable so local
	// (non-Docker) runs don't ever touch a real host /data directory.
	DataDir string
}

// defaultClaudeModel is applied when app_config has no llm.model row.
// Unlike APIKey/BaseURL there is a sane default here — a missing model
// isn't "unconfigured" the way a missing key is, it's just every Claude
// call 404ing with a confusing "model: " error until someone notices and
// inserts the row by hand.
const defaultClaudeModel = "claude-sonnet-5"

// LLM is the resolved Tier-2 LLM configuration, read from app_config with
// code defaults applied for keys that have one. APIKey and BaseURL default
// to "" (unconfigured) — there is no sane default API key, and
// dev-property-finder's precedent is to leave a provider unconfigured
// rather than guess (see docs/spec/00_infrastructure_setup/00-03
// -llm-agent-integration.md "Config"). Model defaults to defaultClaudeModel.
type LLM struct {
	Provider            string
	APIKey              string
	BaseURL             string
	Model               string
	TimeoutSeconds      int
	MaxRetries          int
	MaxToolCallsPerTurn int
	MaxLLMRoundtrips    int
	ReasoningLanguage   string
}

// String redacts APIKey so a stray %v/%+v of an LLM value in a log line
// can never leak the key (it is write-only over HTTP, docs/spec 00-03
// "Config").
func (l LLM) String() string {
	return fmt.Sprintf("{provider=%s model=%q base_url=%q timeout_seconds=%d max_retries=%d max_tool_calls_per_turn=%d max_llm_roundtrips=%d reasoning_language=%s api_key_configured=%t}",
		l.Provider, l.Model, l.BaseURL, l.TimeoutSeconds, l.MaxRetries, l.MaxToolCallsPerTurn, l.MaxLLMRoundtrips, l.ReasoningLanguage, l.APIKey != "")
}

// GoString makes %#v redact the key the same way String does.
func (l LLM) GoString() string { return l.String() }

// LoadEnv reads Env from environ, a slice of "KEY=VALUE" strings such as
// os.Environ(). DATABASE_URL and APP_PASSWORD are required; HTTP_LISTEN_ADDR
// defaults to ":8080" when absent or blank; COOKIE_SECURE defaults to true.
func LoadEnv(environ []string) (Env, error) {
	values := parseEnviron(environ)

	databaseURL := strings.TrimSpace(values["DATABASE_URL"])
	if databaseURL == "" {
		return Env{}, fmt.Errorf("DATABASE_URL is required")
	}

	appPassword := strings.TrimSpace(values["APP_PASSWORD"])
	if appPassword == "" {
		return Env{}, fmt.Errorf("APP_PASSWORD is required")
	}
	cookieSecure := defaultBool(values["COOKIE_SECURE"], true)

	return Env{
		DatabaseURL:    databaseURL,
		HTTPListenAddr: envOrDefault(values, "HTTP_LISTEN_ADDR", ":8080"),
		ClaudeAPIKey:   strings.TrimSpace(values["CLAUDE_API_KEY"]),
		AppPassword:    appPassword,
		CookieSecure:   cookieSecure,
		DataDir:        envOrDefault(values, "DATA_DIR", "/data"),
	}, nil
}

// ConnectPool builds a pgxpool connected to databaseURL with sane defaults
// for a single-user, local-network app.
func ConnectPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	poolConfig.MaxConnIdleTime = 5 * time.Minute
	poolConfig.MaxConnLifetime = time.Hour

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if pingError := pool.Ping(ctx); pingError != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", pingError)
	}
	return pool, nil
}

// SeedFromEnv writes claudeAPIKey into app_config as llm.api_key when no
// such key exists yet. Once seeded, the DB value is authoritative — the
// env var is not consulted again on subsequent starts. A blank
// claudeAPIKey is a no-op (the LLM subsystem simply stays unconfigured).
func SeedFromEnv(ctx context.Context, pool *pgxpool.Pool, claudeAPIKey string) error {
	if strings.TrimSpace(claudeAPIKey) == "" {
		return nil
	}
	const insertSQL = `
INSERT INTO app_config (key, category, value, updated_at)
SELECT 'llm.api_key', 'llm', $1, NOW()
WHERE NOT EXISTS (SELECT 1 FROM app_config WHERE key = 'llm.api_key')
`
	if _, err := pool.Exec(ctx, insertSQL, claudeAPIKey); err != nil {
		return fmt.Errorf("seed llm.api_key: %w", err)
	}
	return nil
}

// execer is the one method SaveLLM needs; *pgxpool.Pool and pgx.Tx both satisfy it.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// SaveLLM upserts every editable llm.* key of cfg into app_config in one
// statement, so a failure never leaves a half-written configuration. The
// API key row is written only when includeAPIKey is true: the key is
// write-only over HTTP, so an update that does not carry a new key must
// leave the stored one untouched. Rows not managed here (anything outside
// the keys below) are not touched.
func SaveLLM(ctx context.Context, db execer, cfg LLM, includeAPIKey bool) error {
	entries := map[string]string{
		"llm.provider":                cfg.Provider,
		"llm.base_url":                cfg.BaseURL,
		"llm.model":                   cfg.Model,
		"llm.timeout_seconds":         strconv.Itoa(cfg.TimeoutSeconds),
		"llm.max_retries":             strconv.Itoa(cfg.MaxRetries),
		"llm.max_tool_calls_per_turn": strconv.Itoa(cfg.MaxToolCallsPerTurn),
		"llm.max_llm_roundtrips":      strconv.Itoa(cfg.MaxLLMRoundtrips),
		"llm.reasoning_language":      cfg.ReasoningLanguage,
	}
	if includeAPIKey {
		entries["llm.api_key"] = cfg.APIKey
	}
	keys := make([]string, 0, len(entries))
	values := make([]string, 0, len(entries))
	for key, value := range entries {
		keys = append(keys, key)
		values = append(values, value)
	}
	const upsertSQL = `
INSERT INTO app_config (key, category, value, updated_at)
SELECT k, 'llm', v, NOW() FROM unnest($1::text[], $2::text[]) AS t(k, v)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
`
	if _, err := db.Exec(ctx, upsertSQL, keys, values); err != nil {
		return fmt.Errorf("save llm config: %w", err)
	}
	return nil
}

// LoadAll reads every Tier 2 llm.* key from app_config and resolves the
// full LLM config, applying code defaults for keys that have one (see
// docs/spec/00_infrastructure_setup/00-03-llm-agent-integration.md
// "Config"). Provider defaults to "claude" (the sole MVP provider per ADR
// 0006) even when app_config has no row for it yet.
func LoadAll(ctx context.Context, pool *pgxpool.Pool) (LLM, error) {
	const selectSQL = `SELECT key, value FROM app_config WHERE category = 'llm'`
	rows, err := pool.Query(ctx, selectSQL)
	if err != nil {
		return LLM{}, fmt.Errorf("load llm config: %w", err)
	}
	defer rows.Close()

	raw := make(map[string]string)
	for rows.Next() {
		var key, value string
		if scanError := rows.Scan(&key, &value); scanError != nil {
			return LLM{}, fmt.Errorf("scan llm config row: %w", scanError)
		}
		raw[key] = value
	}
	if rowsError := rows.Err(); rowsError != nil {
		return LLM{}, fmt.Errorf("iterate llm config rows: %w", rowsError)
	}

	return LLM{
		Provider:            defaultString(raw["llm.provider"], "claude"),
		APIKey:              raw["llm.api_key"],
		BaseURL:             raw["llm.base_url"],
		Model:               defaultString(raw["llm.model"], defaultClaudeModel),
		TimeoutSeconds:      defaultInt(raw["llm.timeout_seconds"], 30),
		MaxRetries:          defaultInt(raw["llm.max_retries"], 2),
		MaxToolCallsPerTurn: defaultInt(raw["llm.max_tool_calls_per_turn"], 15),
		MaxLLMRoundtrips:    defaultInt(raw["llm.max_llm_roundtrips"], 5),
		ReasoningLanguage:   defaultString(raw["llm.reasoning_language"], "de"),
	}, nil
}

func defaultString(raw, fallback string) string {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	return raw
}

func defaultInt(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

// defaultBool parses raw with strconv.ParseBool, falling back to fallback
// when raw is blank or unparseable — mirrors defaultInt's shape.
func defaultBool(raw string, fallback bool) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
}

func parseEnviron(environ []string) map[string]string {
	values := make(map[string]string, len(environ))
	for _, entry := range environ {
		key, value, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		values[key] = value
	}
	return values
}

func envOrDefault(values map[string]string, key, fallback string) string {
	if value := strings.TrimSpace(values[key]); value != "" {
		return value
	}
	return fallback
}
