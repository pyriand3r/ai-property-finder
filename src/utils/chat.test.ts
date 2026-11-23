import { describe, expect, it } from 'vitest';

import { confirmableCalls, decodeUnicodeEscapes, hasPendingConfirmCard } from '@/utils/chat';

import type { ChatMessage, StoredToolCall } from '@/types/chat';

function message(toolCalls?: StoredToolCall[]): ChatMessage {
  return {
    id: 'm1',
    session_id: 's1',
    role: 'assistant',
    content: '',
    tool_calls: toolCalls,
    created_at: '2026-01-01T00:00:00Z',
  };
}

describe('decodeUnicodeEscapes', () => {
  it('decodes literal \\uXXXX sequences left by a double-escaping model', () => {
    expect(decodeUnicodeEscapes(String.raw`K\u00fcche, Bad und Gr\u00f6\u00dfe`)).toBe(
      'Küche, Bad und Größe',
    );
  });

  it('accepts upper- and lower-case hex digits', () => {
    expect(decodeUnicodeEscapes(String.raw`\u00FC\u00fc`)).toBe('üü');
  });

  it('decodes surrogate pairs half by half into the real character', () => {
    expect(decodeUnicodeEscapes(String.raw`\ud83c\udfe0`)).toBe('🏠');
  });

  it('leaves text without escapes and incomplete escapes untouched', () => {
    expect(decodeUnicodeEscapes('Haus am See')).toBe('Haus am See');
    expect(decodeUnicodeEscapes(String.raw`\u00f`)).toBe(String.raw`\u00f`);
  });
});

describe('confirmableCalls', () => {
  it('keeps only Confirm-Card tools and preserves their original tool_calls index', () => {
    const calls: StoredToolCall[] = [
      { id: 'a', name: 'fetch_url', input: {} },
      { id: 'b', name: 'propose_criteria', input: {} },
      { id: 'c', name: 'save_listing', input: {} },
      { id: 'd', name: 'update_context', input: {} },
    ];
    expect(confirmableCalls(message(calls)).map(({ call, index }) => [call.id, index])).toEqual([
      ['b', 1],
      ['d', 3],
    ]);
  });

  it('returns an empty list when the message has no tool calls', () => {
    expect(confirmableCalls(message())).toEqual([]);
  });
});

describe('hasPendingConfirmCard', () => {
  it('is true while a Confirm-Card call has no status (ADR 0017)', () => {
    expect(
      hasPendingConfirmCard(
        message([
          { id: 'a', name: 'propose_criteria', input: {}, status: 'confirmed' },
          { id: 'b', name: 'update_context', input: {} },
        ]),
      ),
    ).toBe(true);
  });

  it('is false once every Confirm-Card call is confirmed or rejected', () => {
    expect(
      hasPendingConfirmCard(
        message([
          { id: 'a', name: 'propose_criteria', input: {}, status: 'confirmed' },
          { id: 'b', name: 'update_context', input: {}, status: 'rejected' },
        ]),
      ),
    ).toBe(false);
  });

  it('ignores pending non-confirmable tool calls', () => {
    expect(hasPendingConfirmCard(message([{ id: 'a', name: 'fetch_url', input: {} }]))).toBe(false);
  });
});
