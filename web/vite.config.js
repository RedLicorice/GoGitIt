import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
  plugins: [svelte()],
  server: {
    // Bind all interfaces so teammates can reach the dev server over Tailscale.
    host: true,
    // Accept any Host header — teammates reach this by Tailscale machine name
    // (e.g. my-dev-box) or MagicDNS FQDN, which Vite blocks by default.
    // Safe here: dev server on a private tailnet.
    allowedHosts: true,
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        ws: true, // proxy the /api/v1/ws live-updates WebSocket
      },
      '/auth': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
});
