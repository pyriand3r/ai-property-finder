import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';

import LlmConfigForm from './LlmConfigForm.vue';

import type { AppConfig } from '@/types/config';

function config(provider: string, base_url: string): AppConfig {
  return {
    llm: {
      provider,
      base_url,
      model: 'qwen3',
      timeout_seconds: 30,
      max_retries: 2,
      max_tool_calls_per_turn: 15,
      max_llm_roundtrips: 5,
      reasoning_language: 'de',
      api_key_configured: false,
    },
    providers: [
      {
        id: 'claude',
        label: 'Claude (Anthropic)',
        models: [],
        requires_base_url: false,
        default_base_url: '',
        requires_api_key: true,
      },
      {
        id: 'lmstudio',
        label: 'LM Studio (lokal)',
        models: [],
        requires_base_url: true,
        default_base_url: 'http://localhost:1234/v1',
        requires_api_key: false,
      },
    ],
  };
}

describe('LlmConfigForm', () => {
  it('blocks saving a provider without built-in endpoint until a Base URL is set', () => {
    const wrapper = mount(LlmConfigForm, {
      props: { config: config('lmstudio', ''), saving: false },
    });

    expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeDefined();
    expect(wrapper.text()).toContain('API-Key (optional)');
  });

  it('allows saving a provider without Base URL when the backend does not require one', () => {
    const wrapper = mount(LlmConfigForm, {
      props: { config: config('claude', ''), saving: false },
    });

    expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeUndefined();
  });
});
