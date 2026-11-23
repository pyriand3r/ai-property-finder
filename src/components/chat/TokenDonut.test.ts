import { mount } from '@vue/test-utils';
import { NProgress } from 'naive-ui';
import { describe, expect, it } from 'vitest';

import TokenDonut from '@/components/chat/TokenDonut.vue';

function progress(tokenCountEstimate?: number) {
  const wrapper = mount(TokenDonut, { props: { tokenCountEstimate } });
  return wrapper.findComponent(NProgress);
}

describe('TokenDonut', () => {
  it('shows 0 % and success status for an empty session', () => {
    const bar = progress();
    expect(bar.props('percentage')).toBe(0);
    expect(bar.props('status')).toBe('success');
  });

  it('turns to warning at 60 % of the 200k context window', () => {
    const bar = progress(120_000);
    expect(bar.props('percentage')).toBe(60);
    expect(bar.props('status')).toBe('warning');
  });

  it('turns to error at 80 %', () => {
    const bar = progress(160_000);
    expect(bar.props('percentage')).toBe(80);
    expect(bar.props('status')).toBe('error');
  });

  it('caps the percentage at 100 when the estimate exceeds the window', () => {
    expect(progress(500_000).props('percentage')).toBe(100);
  });
});
