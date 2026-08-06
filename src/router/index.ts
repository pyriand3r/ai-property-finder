import { createRouter, createWebHistory } from 'vue-router';

export const router = createRouter({
  history: createWebHistory(),
  routes: [
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
  ],
});
