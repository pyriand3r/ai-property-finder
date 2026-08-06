import { http } from '@/api/http';

import type { ContextFile, ContextRevision } from '@/types/context';

export function getGlobalContext() {
  return http.get<ContextFile>('/api/context/global');
}

export function updateGlobalContext(contentMd: string) {
  return http.put<ContextFile>('/api/context/global', { content_md: contentMd });
}

export function getGlobalContextHistory() {
  return http.get<ContextRevision[]>('/api/context/global/history');
}

export function restoreGlobalContext(revisionId: string) {
  return http.post<ContextFile>(`/api/context/global/revisions/${revisionId}/restore`);
}

export function getProfileContext(profileId: string) {
  return http.get<ContextFile>(`/api/profiles/${profileId}/context`);
}

export function updateProfileContext(profileId: string, contentMd: string) {
  return http.put<ContextFile>(`/api/profiles/${profileId}/context`, { content_md: contentMd });
}

export function getProfileContextHistory(profileId: string) {
  return http.get<ContextRevision[]>(`/api/profiles/${profileId}/context/history`);
}

export function restoreProfileContext(profileId: string, revisionId: string) {
  return http.post<ContextFile>(
    `/api/profiles/${profileId}/context/revisions/${revisionId}/restore`,
  );
}
