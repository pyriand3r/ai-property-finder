/** Mirrors backend/internal/api/config_handler.go llmConfigResponse. The API key is
 * write-only: the backend only reports whether one is set. */
export interface LlmConfig {
  provider: string;
  base_url: string;
  model: string;
  timeout_seconds: number;
  max_retries: number;
  max_tool_calls_per_turn: number;
  max_llm_roundtrips: number;
  reasoning_language: string;
  api_key_configured: boolean;
}

/** Mirrors backend/internal/llm.ProviderInfo. `models` are suggestions, not an allow-list. */
export interface ProviderInfo {
  id: string;
  label: string;
  models: string[];
  requires_base_url: boolean;
}

/** Mirrors GET/PATCH /api/config. */
export interface AppConfig {
  llm: LlmConfig;
  providers: ProviderInfo[];
}

/** Mirrors backend/internal/llm.Update: every field optional. `api_key` is only sent
 * when the user typed a new one — omitted means "keep the stored key". */
export interface LlmConfigUpdate {
  provider?: string;
  api_key?: string;
  base_url?: string;
  model?: string;
  timeout_seconds?: number;
  max_retries?: number;
  max_tool_calls_per_turn?: number;
  max_llm_roundtrips?: number;
  reasoning_language?: string;
}
