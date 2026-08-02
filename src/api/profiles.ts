import { http } from '@/api/http';

import type { Profile } from '@/types/profile';

export function listProfiles() {
  return http.get<Profile[]>('/api/profiles');
}

export function getProfile(profileId: string) {
  return http.get<Profile>(`/api/profiles/${profileId}`);
}

export function createProfile(name: string) {
  return http.post<Profile>('/api/profiles', { name });
}

export function renameProfile(profileId: string, name: string) {
  return http.patch<Profile>(`/api/profiles/${profileId}`, { name });
}

export function archiveProfile(profileId: string) {
  return http.patch<Profile>(`/api/profiles/${profileId}/archive`);
}

export function deleteProfile(profileId: string) {
  return http.delete<void>(`/api/profiles/${profileId}`);
}
