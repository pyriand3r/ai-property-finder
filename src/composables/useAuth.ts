import { readonly, shallowRef } from 'vue';

import * as authApi from '@/api/auth';

/**
 * Module-level (not per-call) state: unlike this file's sibling
 * composables, login status is one process-wide flag every consumer
 * (router guard, App.vue logout button, LoginView) must observe as the
 * same value — see docs/adr/0018. null = not checked yet this page load.
 */
const isAuthenticated = shallowRef<boolean | null>(null);
const checking = shallowRef(false);

export function useAuth() {
  /** Calls GET /api/auth/status; any failure (401 or network) sets
   * isAuthenticated to false. Returns the resolved value so router guards
   * can await it directly. */
  async function checkStatus(): Promise<boolean> {
    checking.value = true;
    try {
      await authApi.status();
      isAuthenticated.value = true;
    } catch {
      isAuthenticated.value = false;
    } finally {
      checking.value = false;
    }
    return isAuthenticated.value;
  }

  async function login(password: string): Promise<void> {
    await authApi.login(password);
    isAuthenticated.value = true;
  }

  async function logout(): Promise<void> {
    await authApi.logout();
    isAuthenticated.value = false;
  }

  return {
    isAuthenticated: readonly(isAuthenticated),
    checking: readonly(checking),
    checkStatus,
    login,
    logout,
  };
}
