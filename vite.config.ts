import { fileURLToPath, URL } from 'node:url';

import vue from '@vitejs/plugin-vue';
import { defineConfig } from 'vite';

// Dev server proxies /api and /media to the Go backend (docker-compose.yml,
// AGENTS.md "Dev Commands"). Production build is embedded into the backend
// binary (backend/internal/web) — see Layer 4.2 in docs/implementation-plan.md.
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
      '/media': 'http://localhost:8080',
    },
  },
  build: {
    outDir: 'dist',
  },
});
