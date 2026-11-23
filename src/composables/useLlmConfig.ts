import { readonly, shallowRef } from 'vue';

import * as configApi from '@/api/config';

import type { AppConfig, LlmConfigUpdate } from '@/types/config';

/** Loads and edits the Tier-2 LLM settings (backend/internal/api/config_handler.go,
 * docs/spec 00-03 "Config"). The API key is write-only — `config.llm` only carries
 * `api_key_configured`. */
export function useLlmConfig() {
  const config = shallowRef<AppConfig | null>(null);
  const loading = shallowRef(false);
  const saving = shallowRef(false);
  const error = shallowRef<string | null>(null);

  async function fetchConfig() {
    loading.value = true;
    error.value = null;
    try {
      config.value = await configApi.getConfig();
    } catch (fetchError) {
      error.value = fetchError instanceof Error ? fetchError.message : String(fetchError);
    } finally {
      loading.value = false;
    }
  }

  /** Rethrows so the caller can show a toast; `error` stays reserved for the load lifecycle. */
  async function save(update: LlmConfigUpdate) {
    saving.value = true;
    try {
      config.value = await configApi.updateLlmConfig(update);
    } finally {
      saving.value = false;
    }
  }

  return {
    config: readonly(config),
    loading: readonly(loading),
    saving: readonly(saving),
    error: readonly(error),
    fetchConfig,
    save,
  };
}
