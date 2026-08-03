import { http } from '@/api/http';

/** Applies a pending propose_criteria/update_context proposal (ADR 0009).
 * Returns the written *domain.Criterion or *domain.Context — callers that
 * only need success/failure can ignore the body. */
export function confirmToolCall(messageId: string, callIndex: number) {
  return http.post<unknown>(`/api/chat/messages/${messageId}/tool-calls/${callIndex}/confirm`);
}

/** Discards a pending proposal — no state change, exists so the frontend
 * has a clean call to dismiss the Confirm-Card (ADR 0009). */
export function rejectToolCall(messageId: string, callIndex: number) {
  return http.post<void>(`/api/chat/messages/${messageId}/tool-calls/${callIndex}/reject`);
}
