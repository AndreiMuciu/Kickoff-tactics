import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The browser only ever talks to the Vite origin; /api is proxied to the Go
// server, so cookies stay same-origin and no CORS is needed.
export default defineConfig({
  plugins: [react()],
  server: {
    host: '127.0.0.1',
    port: 5173,
    strictPort: true,
    // ws: the WebSocket at /api/ws goes through the same proxy.
    proxy: { '/api': { target: 'http://127.0.0.1:8080', changeOrigin: false, ws: true } },
  },
})
