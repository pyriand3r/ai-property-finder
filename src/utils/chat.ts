import type { ChatMessage, StoredToolCall } from '@/types/chat';

/** Tool calls that render as a Confirm-Card (ADR 0009) — shared between
 * ChatMessageList.vue (renders them) and useChat.ts (detects whether any
 * are still pending, for the "Neue Session" warning, Layer 8.3). */
export const CONFIRMABLE_TOOLS = new Set(['propose_criteria', 'update_context']);

/** Confirm-Card-worthy tool calls, paired with their index in tool_calls —
 * the confirm/reject endpoint addresses a call by that array index, so it
 * must survive filtering (spec 00-03/ADR 0009, Layer 6.5). */
export function confirmableCalls(message: ChatMessage): { call: StoredToolCall; index: number }[] {
  return (message.tool_calls ?? [])
    .map((call, index) => ({ call, index }))
    .filter(({ call }) => CONFIRMABLE_TOOLS.has(call.name));
}

/** True if message has at least one Confirm-Card-worthy tool call the user
 * hasn't confirmed or rejected yet (ADR 0017: status unset = pending). */
export function hasPendingConfirmCard(message: ChatMessage): boolean {
  return confirmableCalls(message).some(({ call }) => !call.status);
}
