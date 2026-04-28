import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': {
        target: process.env.FUNNELBARN_API_URL || 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
