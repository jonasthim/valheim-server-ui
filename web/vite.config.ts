import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// Dev server proxies the API to the Go backend (make dev-backend on :8080).
export default defineConfig({
  plugins: [react()],
  build: { outDir: 'dist', emptyOutDir: true, sourcemap: false },
  server: {
    port: 5173,
    proxy: {
      '/api': { target: 'http://127.0.0.1:8080', changeOrigin: false },
    },
  },
})
