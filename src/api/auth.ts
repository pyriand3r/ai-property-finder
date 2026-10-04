import { http } from '@/api/http';

export function login(password: string) {
  return http.post<void>('/api/auth/login', { password });
}

export function logout() {
  return http.post<void>('/api/auth/logout');
}

export function status() {
  return http.get<{ authenticated: boolean }>('/api/auth/status');
}
