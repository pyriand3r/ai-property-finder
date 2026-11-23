import { createRouter, createWebHistory } from 'vue-router';

import { useAuth } from '@/composables/useAuth';

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue'),
    },
    {
      path: '/',
      name: 'profile-list',
      component: () => import('@/views/ProfileListView.vue'),
    },
    {
      path: '/profiles/:profileId',
      name: 'profile-shell',
      component: () => import('@/views/ProfileShellView.vue'),
      props: true,
    },
    {
      path: '/listings/:listingId',
      name: 'listing-detail',
      component: () => import('@/views/ListingDetailView.vue'),
      props: true,
    },
    {
      path: '/context',
      name: 'global-context',
      component: () => import('@/views/GlobalContextView.vue'),
    },
    {
      path: '/settings',
      name: 'settings',
      component: () => import('@/views/SettingsView.vue'),
    },
  ],
});

router.beforeEach(async (to) => {
  if (to.name === 'login') {
    return true;
  }
  const { isAuthenticated, checkStatus } = useAuth();
  if (isAuthenticated.value === null) {
    await checkStatus();
  }
  if (!isAuthenticated.value) {
    return { name: 'login', query: { redirect: to.fullPath } };
  }
  return true;
});
