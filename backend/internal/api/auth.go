package api

// auth.go implements password-based login gating this single-user app
// (docs/adr/0018). No SearchProfile/user table — one password from
// config.Env.AppPassword, compared in constant time; its SHA-256 digest is
// the session JWT's HMAC key.

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"propertyfinder/internal/auth"
	"strings"
	"time"
)

const sessionCookieName = "pf_session"

// sessionTTL is the session JWT's lifetime — see docs/adr/0018 ("30 Tage,
// passt zur Selbsthosting/Local-Network-Nutzung").
const sessionTTL = 30 * 24 * time.Hour

// AuthHandler issues/validates the session cookie. RequireAuth is the chi
// middleware every protected route group uses (router.go).
type AuthHandler struct {
	password     string
	secret       [sha256.Size]byte
	cookieSecure bool
}

// NewAuthHandler derives the JWT signing secret from password's SHA-256
// digest — see docs/adr/0018 for why no separate signing key exists.
// cookieSecure sets the session cookie's Secure flag (config.Env.CookieSecure).
func NewAuthHandler(password string, cookieSecure bool) *AuthHandler {
	return &AuthHandler{
		password:     password,
		secret:       sha256.Sum256([]byte(password)),
		cookieSecure: cookieSecure,
	}
}

type loginRequest struct {
	Password string `json:"password"`
}

// Login handles POST /api/auth/login.
func (handler *AuthHandler) Login(writer http.ResponseWriter, request *http.Request) {
	var body loginRequest
	if !decodeJSON(writer, request, &body) {
		return
	}
	if subtle.ConstantTimeCompare([]byte(body.Password), []byte(handler.password)) != 1 {
		writeJSON(writer, http.StatusUnauthorized, apiError{Error: "Falsches Passwort"})
		return
	}
	token, err := auth.IssueToken(handler.secret[:], sessionTTL)
	if err != nil {
		writeError(writer, err)
		return
	}
	http.SetCookie(writer, handler.cookie(token, int(sessionTTL.Seconds())))
	writer.WriteHeader(http.StatusNoContent)
}

// Logout handles POST /api/auth/logout — clears the session cookie.
func (handler *AuthHandler) Logout(writer http.ResponseWriter, _ *http.Request) {
	http.SetCookie(writer, handler.cookie("", -1))
	writer.WriteHeader(http.StatusNoContent)
}

// Status handles GET /api/auth/status. It is mounted behind RequireAuth,
// so reaching this handler at all already proves the session is valid —
// the body only exists so the frontend has a stable JSON shape to parse.
func (handler *AuthHandler) Status(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]bool{"authenticated": true})
}

func (handler *AuthHandler) cookie(value string, maxAgeSeconds int) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAgeSeconds,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		// Secure is driven by config.Env.CookieSecure (docs/adr/0018):
		// true by default because the documented deployment sits behind
		// an external TLS-terminating reverse proxy (the container only
		// ever speaks plain HTTP itself); set COOKIE_SECURE=false for
		// local development without a proxy in front.
		Secure: handler.cookieSecure,
	}
}

// RequireAuth is chi middleware: a missing/invalid/expired session cookie
// yields 401 before the wrapped handler runs. GET /health, POST
// /api/auth/login, and the embedded SPA's static assets (router.go) are
// mounted outside every group using this middleware — the SPA shell
// (including the login screen itself) must load unauthenticated.
func (handler *AuthHandler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie(sessionCookieName)
		if err != nil || auth.VerifyToken(handler.secret[:], cookie.Value) != nil {
			writeJSON(writer, http.StatusUnauthorized, apiError{Error: "unauthorized"})
			return
		}
		next.ServeHTTP(writer, request)
	})
}

// RequireBearer is chi middleware gating the MCP endpoint (docs/adr/0019)
// with a static bearer token — the same APP_PASSWORD secret RequireAuth's
// cookie carries, checked via the Authorization header instead, since MCP
// clients (Claude Code, opencode, mcp-remote for Claude Desktop) cannot
// perform an interactive cookie login.
func (handler *AuthHandler) RequireBearer(next http.Handler) http.Handler {
	const prefix = "Bearer "
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		header := request.Header.Get("Authorization")
		token := strings.TrimPrefix(header, prefix)
		if !strings.HasPrefix(header, prefix) || subtle.ConstantTimeCompare([]byte(token), []byte(handler.password)) != 1 {
			writer.Header().Set("WWW-Authenticate", `Bearer realm="property-finder"`)
			writeJSON(writer, http.StatusUnauthorized, apiError{Error: "unauthorized"})
			return
		}
		next.ServeHTTP(writer, request)
	})
}
