import { http } from '@/api/http';

import type { ChatMessage, ChatSession } from '@/types/chat';

export function listSessions(profileId: string) {
  return http.get<ChatSession[]>(`/api/profiles/${profileId}/chat/sessions`);
}

/** Ends the profile's current active session and starts a fresh one
 * (ADR 0014 "Neue Session" button). */
export function startSession(profileId: string) {
  return http.post<ChatSession>(`/api/profiles/${profileId}/chat/sessions`);
}

export function listMessages(profileId: string, sessionId: string) {
  return http.get<ChatMessage[]>(`/api/profiles/${profileId}/chat/sessions/${sessionId}/messages`);
}

/** Persists a user message. Layer 6.1: no Agent yet, this is the only
 * message role written. Rejected with 409 if sessionId is no longer the
 * active session (backend enforces read-only history). */
export function sendMessage(
  profileId: string,
  sessionId: string,
  content: string,
  attachmentPath?: string,
) {
  return http.post<ChatMessage>(`/api/profiles/${profileId}/chat/sessions/${sessionId}/messages`, {
    content,
    attachment_path: attachmentPath,
  });
}

/** Cancels the session's in-flight Agent turn (spec 01-01 "stop" control,
 * Layer 8.4). 404 if no turn is currently running — a harmless race with
 * the turn ending on its own that callers can ignore. */
export function stopTurn(profileId: string, sessionId: string) {
  return http.post<void>(`/api/profiles/${profileId}/chat/sessions/${sessionId}/stop`);
}
