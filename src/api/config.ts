import { http } from '@/api/http';

import type { AppConfig, LlmConfigUpdate } from '@/types/config';

export function getConfig() {
  return http.get<AppConfig>('/api/config');
}

/** Partial update of the LLM settings; takes effect for the next chat turn / evaluation. */
export function updateLlmConfig(update: LlmConfigUpdate) {
  return http.patch<AppConfig>('/api/config', { llm: update });
}
