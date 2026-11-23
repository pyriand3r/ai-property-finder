import type { DuplicateListing } from '@/types/listing';

/**
 * Wire shape of any non-2xx response body (backend/internal/api/helpers.go).
 * `duplicates` is set only on the 409 "duplicate_listing" warning (issue #10).
 */
interface ApiErrorBody {
  error: string;
  duplicates?: DuplicateListing[];
}

export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
    public readonly duplicates?: DuplicateListing[],
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  });

  if (!response.ok) {
    const body = (await response.json().catch(() => null)) as ApiErrorBody | null;
    if (response.status === 401) {
      // Dynamic import, not a top-level one: src/router/index.ts's auth
      // guard transitively imports this module (via useAuth -> api/auth
      // -> http), so a static top-level `import { router }` here would
      // be a real import cycle (unlike the router's lazily-loaded view
      // components, which don't create one).
      const { router } = await import('@/router');
      if (router.currentRoute.value.name !== 'login') {
        void router.push({ name: 'login' });
      }
    }
    throw new ApiError(body?.error ?? response.statusText, response.status, body?.duplicates);
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return (await response.json()) as T;
}

export const http = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined }),
  patch: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PATCH', body: body ? JSON.stringify(body) : undefined }),
  put: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PUT', body: body ? JSON.stringify(body) : undefined }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
};
