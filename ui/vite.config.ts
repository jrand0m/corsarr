import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
  plugins: [svelte()],
  base: './',
  build: {
    outDir: '../internal/web/dist',
    emptyOutDir: true,
    target: 'es2022',
    cssCodeSplit: false,
    reportCompressedSize: true,
  },
  server: { proxy: { '/api': 'http://localhost:8090' } },
  test: { environment: 'jsdom', include: ['src/**/*.test.ts'], setupFiles: ['src/test-setup.ts'], testTimeout: process.env.CI ? 30000 : 5000 },
  resolve: process.env.VITEST ? { conditions: ['browser'] } : undefined,
});
