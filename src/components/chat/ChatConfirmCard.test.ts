import { mount } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';

import ChatConfirmCard from '@/components/chat/ChatConfirmCard.vue';

vi.mock('@/api/toolCalls', () => ({
  confirmToolCall: vi.fn().mockResolvedValue(undefined),
  rejectToolCall: vi.fn().mockResolvedValue(undefined),
}));

function card(input: object) {
  return mount(ChatConfirmCard, {
    props: { messageId: 'm1', callIndex: 0, toolName: 'propose_criteria', input },
  });
}

describe('ChatConfirmCard', () => {
  it('offers to activate a newly proposed criterion (it is already stored as proposed)', () => {
    const wrapper = card({ action: 'add', name: 'Ruhe' });

    expect(wrapper.text()).toContain('Neues Kriterium freischalten?');
    expect(wrapper.text()).toContain('Freischalten');
    expect(wrapper.text()).not.toContain('Übernehmen');
  });

  it('keeps Übernehmen for a change to an existing criterion', () => {
    const wrapper = card({ action: 'edit', criterion_id: 'c1', name: 'Ruhe' });

    expect(wrapper.text()).toContain('Kriterium ändern?');
    expect(wrapper.text()).toContain('Übernehmen');
  });
});
