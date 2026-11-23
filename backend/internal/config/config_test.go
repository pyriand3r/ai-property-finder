package config

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestLoadEnv_RequiredValues(t *testing.T) {
	tests := map[string]struct {
		environ []string
		wantErr string
	}{
		"missing DATABASE_URL": {[]string{"APP_PASSWORD=pw"}, "DATABASE_URL"},
		"blank DATABASE_URL":   {[]string{"DATABASE_URL=   ", "APP_PASSWORD=pw"}, "DATABASE_URL"},
		"missing APP_PASSWORD": {[]string{"DATABASE_URL=postgres://x"}, "APP_PASSWORD"},
		"blank APP_PASSWORD":   {[]string{"DATABASE_URL=postgres://x", "APP_PASSWORD= "}, "APP_PASSWORD"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := LoadEnv(test.environ)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("err = %v, want one naming %s", err, test.wantErr)
			}
		})
	}
}

func TestLoadEnv_AppliesDefaults(t *testing.T) {
	env, err := LoadEnv([]string{"DATABASE_URL=postgres://x", "APP_PASSWORD=pw", "malformed-entry"})
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}

	want := Env{DatabaseURL: "postgres://x", HTTPListenAddr: ":8080", AppPassword: "pw", CookieSecure: true, DataDir: "/data"}
	if env != want {
		t.Errorf("got %+v, want %+v", env, want)
	}
}

func TestLoadEnv_ReadsOverridesAndTrims(t *testing.T) {
	env, err := LoadEnv([]string{
		"DATABASE_URL= postgres://x ", "APP_PASSWORD= pw ", "HTTP_LISTEN_ADDR=:9000",
		"CLAUDE_API_KEY= sk-1 ", "DATA_DIR=/tmp/pf", "COOKIE_SECURE=false",
	})
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}

	want := Env{DatabaseURL: "postgres://x", HTTPListenAddr: ":9000", ClaudeAPIKey: "sk-1", AppPassword: "pw", CookieSecure: false, DataDir: "/tmp/pf"}
	if env != want {
		t.Errorf("got %+v, want %+v", env, want)
	}
}

// TestLoadEnv_ValueMayContainEquals: passwords/URLs routinely contain '='.
func TestLoadEnv_ValueMayContainEquals(t *testing.T) {
	env, err := LoadEnv([]string{"DATABASE_URL=postgres://x?sslmode=disable", "APP_PASSWORD=a=b"})
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if env.DatabaseURL != "postgres://x?sslmode=disable" || env.AppPassword != "a=b" {
		t.Errorf("got %+v", env)
	}
}

// TestLoadEnv_UnparseableCookieSecureStaysSecure: a typo must not silently
// downgrade the cookie to insecure (ADR 0018).
func TestLoadEnv_UnparseableCookieSecureStaysSecure(t *testing.T) {
	env, err := LoadEnv([]string{"DATABASE_URL=x", "APP_PASSWORD=pw", "COOKIE_SECURE=nope"})
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if !env.CookieSecure {
		t.Error("CookieSecure = false for an unparseable value, want the secure default")
	}
}

func TestDefaultInt(t *testing.T) {
	tests := []struct {
		raw  string
		want int
	}{{"", 7}, {"  ", 7}, {"abc", 7}, {"12", 12}, {" 12 ", 12}, {"0", 0}, {"-3", -3}}
	for _, test := range tests {
		if got := defaultInt(test.raw, 7); got != test.want {
			t.Errorf("defaultInt(%q) = %d, want %d", test.raw, got, test.want)
		}
	}
}

func TestDefaultString_BlankFallsBack(t *testing.T) {
	if got := defaultString("  ", "x"); got != "x" {
		t.Errorf("blank: got %q, want fallback", got)
	}
	if got := defaultString("y", "x"); got != "y" {
		t.Errorf("set: got %q, want y", got)
	}
}

// TestLLM_FormattingRedactsAPIKey: the key is write-only (ADR 0022); no
// verb a logger might use may print it.
func TestLLM_FormattingRedactsAPIKey(t *testing.T) {
	cfg := LLM{Provider: "claude", APIKey: "sk-super-secret", Model: "m"}

	for _, format := range []string{"%v", "%+v", "%#v", "%s"} {
		out := fmt.Sprintf(format, cfg)
		if strings.Contains(out, "sk-super-secret") {
			t.Errorf("%s leaks the API key: %s", format, out)
		}
		if !strings.Contains(out, "api_key_configured=true") {
			t.Errorf("%s does not report the key as configured: %s", format, out)
		}
	}
	if out := (LLM{}).String(); !strings.Contains(out, "api_key_configured=false") {
		t.Errorf("empty key not reported as unconfigured: %s", out)
	}
}

type fakeExecer struct {
	sql  string
	args []any
	err  error
}

func (fake *fakeExecer) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	fake.sql, fake.args = sql, args
	return pgconn.CommandTag{}, fake.err
}

func savedEntries(t *testing.T, fake *fakeExecer) map[string]string {
	t.Helper()
	if len(fake.args) != 2 {
		t.Fatalf("Exec got %d args, want keys and values arrays", len(fake.args))
	}
	keys, okKeys := fake.args[0].([]string)
	values, okValues := fake.args[1].([]string)
	if !okKeys || !okValues || len(keys) != len(values) {
		t.Fatalf("args = %#v, want two equal-length []string", fake.args)
	}
	entries := make(map[string]string, len(keys))
	for i, key := range keys {
		entries[key] = values[i]
	}
	return entries
}

func TestSaveLLM_WritesAllEditableKeysInOneStatement(t *testing.T) {
	fake := &fakeExecer{}
	cfg := LLM{
		Provider: "claude", APIKey: "sk-new", BaseURL: "http://b", Model: "m", TimeoutSeconds: 31,
		MaxRetries: 3, MaxToolCallsPerTurn: 16, MaxLLMRoundtrips: 6, ReasoningLanguage: "en",
	}

	if err := SaveLLM(context.Background(), fake, cfg, false); err != nil {
		t.Fatalf("SaveLLM: %v", err)
	}

	want := map[string]string{
		"llm.provider": "claude", "llm.base_url": "http://b", "llm.model": "m", "llm.timeout_seconds": "31",
		"llm.max_retries": "3", "llm.max_tool_calls_per_turn": "16", "llm.max_llm_roundtrips": "6", "llm.reasoning_language": "en",
	}
	if got := savedEntries(t, fake); !reflect.DeepEqual(got, want) {
		t.Errorf("saved %v, want %v", got, want)
	}
}

// TestSaveLLM_APIKeyOnlyWrittenWhenRequested: an update without a new key
// must leave the stored key untouched (write-only key, ADR 0022).
func TestSaveLLM_APIKeyOnlyWrittenWhenRequested(t *testing.T) {
	without, with := &fakeExecer{}, &fakeExecer{}
	cfg := LLM{APIKey: "sk-new"}

	if err := SaveLLM(context.Background(), without, cfg, false); err != nil {
		t.Fatalf("SaveLLM(false): %v", err)
	}
	if err := SaveLLM(context.Background(), with, cfg, true); err != nil {
		t.Fatalf("SaveLLM(true): %v", err)
	}

	if _, present := savedEntries(t, without)["llm.api_key"]; present {
		t.Error("llm.api_key written although includeAPIKey=false")
	}
	if got := savedEntries(t, with)["llm.api_key"]; got != "sk-new" {
		t.Errorf("llm.api_key = %q, want sk-new", got)
	}
}

func TestSaveLLM_WrapsExecError(t *testing.T) {
	boom := errors.New("boom")

	err := SaveLLM(context.Background(), &fakeExecer{err: boom}, LLM{}, false)

	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want it to wrap %v", err, boom)
	}
}
