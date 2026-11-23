package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newLoginRequest(body string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
}

func sessionCookie(t *testing.T, recorder *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			return cookie
		}
	}
	t.Fatalf("no %s cookie in response", sessionCookieName)
	return nil
}

var okHandler = http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
	writer.WriteHeader(http.StatusOK)
})

func TestAuthHandler_Login_CorrectPasswordSetsHardenedSessionCookie(t *testing.T) {
	handler := NewAuthHandler("hunter2", true)
	recorder := httptest.NewRecorder()

	handler.Login(recorder, newLoginRequest(`{"password":"hunter2"}`))

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
	cookie := sessionCookie(t, recorder)
	if cookie.Value == "" || !cookie.HttpOnly || !cookie.Secure || cookie.Path != "/" || cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie not hardened: %+v", cookie)
	}
	if cookie.MaxAge != int(sessionTTL.Seconds()) {
		t.Errorf("MaxAge = %d, want %d", cookie.MaxAge, int(sessionTTL.Seconds()))
	}
}

// TestAuthHandler_Login_CookieSecureFollowsConfig: COOKIE_SECURE=false must
// reach the cookie, otherwise plain-HTTP dev logins loop (ADR 0018).
func TestAuthHandler_Login_CookieSecureFollowsConfig(t *testing.T) {
	handler := NewAuthHandler("pw", false)
	recorder := httptest.NewRecorder()

	handler.Login(recorder, newLoginRequest(`{"password":"pw"}`))

	if sessionCookie(t, recorder).Secure {
		t.Error("cookie Secure = true with cookieSecure=false")
	}
}

func TestAuthHandler_Login_RejectsBadInput(t *testing.T) {
	tests := map[string]struct {
		body       string
		wantStatus int
	}{
		"wrong password": {`{"password":"nope"}`, http.StatusUnauthorized},
		"empty password": {`{"password":""}`, http.StatusUnauthorized},
		"missing field":  {`{}`, http.StatusUnauthorized},
		"invalid json":   {`not json`, http.StatusBadRequest},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			handler := NewAuthHandler("hunter2", true)
			recorder := httptest.NewRecorder()

			handler.Login(recorder, newLoginRequest(test.body))

			if recorder.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
			for _, cookie := range recorder.Result().Cookies() {
				if cookie.Name == sessionCookieName {
					t.Errorf("session cookie set on a rejected login: %+v", cookie)
				}
			}
		})
	}
}

func TestAuthHandler_Logout_ExpiresCookie(t *testing.T) {
	handler := NewAuthHandler("pw", true)
	recorder := httptest.NewRecorder()

	handler.Logout(recorder, httptest.NewRequest(http.MethodPost, "/api/auth/logout", http.NoBody))

	cookie := sessionCookie(t, recorder)
	if recorder.Code != http.StatusNoContent || cookie.Value != "" || cookie.MaxAge >= 0 {
		t.Errorf("status %d, cookie %+v: want 204 and an expired empty cookie", recorder.Code, cookie)
	}
}

// TestAuthHandler_RequireAuth_AcceptsOnlyOwnSessionTokens covers the
// login → protected-route round trip and the ways a cookie can be invalid.
func TestAuthHandler_RequireAuth_AcceptsOnlyOwnSessionTokens(t *testing.T) {
	handler := NewAuthHandler("pw", true)
	login := httptest.NewRecorder()
	handler.Login(login, newLoginRequest(`{"password":"pw"}`))
	valid := sessionCookie(t, login)

	foreign := httptest.NewRecorder()
	NewAuthHandler("other-password", true).Login(foreign, newLoginRequest(`{"password":"other-password"}`))

	tests := map[string]struct {
		cookie     *http.Cookie
		wantStatus int
	}{
		"no cookie":          {nil, http.StatusUnauthorized},
		"valid session":      {valid, http.StatusOK},
		"garbage token":      {&http.Cookie{Name: sessionCookieName, Value: "garbage"}, http.StatusUnauthorized},
		"tampered signature": {&http.Cookie{Name: sessionCookieName, Value: valid.Value + "x"}, http.StatusUnauthorized},
		"signed by other pw": {sessionCookie(t, foreign), http.StatusUnauthorized},
		"cleared cookie":     {&http.Cookie{Name: sessionCookieName, Value: ""}, http.StatusUnauthorized},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/auth/status", http.NoBody)
			if test.cookie != nil {
				request.AddCookie(test.cookie)
			}
			recorder := httptest.NewRecorder()

			handler.RequireAuth(okHandler).ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
		})
	}
}

func TestAuthHandler_Status_ReportsAuthenticated(t *testing.T) {
	recorder := httptest.NewRecorder()

	NewAuthHandler("pw", true).Status(recorder, httptest.NewRequest(http.MethodGet, "/api/auth/status", http.NoBody))

	if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) != `{"authenticated":true}` {
		t.Errorf("status %d body %q", recorder.Code, recorder.Body)
	}
}

func TestAuthHandler_RequireBearer(t *testing.T) {
	tests := map[string]struct {
		header     string
		wantStatus int
	}{
		"correct token":     {"Bearer pw", http.StatusOK},
		"wrong token":       {"Bearer nope", http.StatusUnauthorized},
		"no header":         {"", http.StatusUnauthorized},
		"wrong scheme":      {"Basic pw", http.StatusUnauthorized},
		"prefix only":       {"Bearer ", http.StatusUnauthorized},
		"token w/o prefix":  {"pw", http.StatusUnauthorized},
		"lowercase bearer":  {"bearer pw", http.StatusUnauthorized},
		"trailing garbage":  {"Bearer pw ", http.StatusUnauthorized},
		"token is prefix":   {"Bearer p", http.StatusUnauthorized},
		"token extends pwd": {"Bearer pwx", http.StatusUnauthorized},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody)
			if test.header != "" {
				request.Header.Set("Authorization", test.header)
			}
			recorder := httptest.NewRecorder()

			NewAuthHandler("pw", true).RequireBearer(okHandler).ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
			if test.wantStatus == http.StatusUnauthorized && recorder.Header().Get("WWW-Authenticate") == "" {
				t.Error("401 lacks WWW-Authenticate challenge")
			}
		})
	}
}
