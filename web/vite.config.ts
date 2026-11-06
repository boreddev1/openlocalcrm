import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';

// Target of the /api and /events proxies. Reads the same E2E_BACKEND_PORT as
// playwright.config.ts, which starts the demo backend on that port — the two must
// move together, otherwise the SPA proxies to a port no listener owns. The
// default 8080 matches the documented port and is what CI uses.
const backendTarget = `http://localhost:${process.env.E2E_BACKEND_PORT ?? '8080'}`;

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, './src'),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: backendTarget,
        changeOrigin: true,
      },
      '/events': {
        target: backendTarget,
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    chunkSizeWarningLimit: 800,
  },
});
