/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  // nginx serves the built UI at /app/ (snip's redirects own every other
  // one-segment path), so every asset URL in the build starts with /app/.
  base: '/app/',
  server: {
    // In development, Vite's own server forwards API calls to snip, so the
    // browser still sees one origin and needs no CORS (chapter 16.6).
    proxy: { '/api': 'http://localhost:8080' },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.{ts,tsx}'], // e2e/ belongs to Playwright, not Vitest
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
  },
});
