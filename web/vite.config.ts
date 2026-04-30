/// <reference types="vitest" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { VitePWA } from 'vite-plugin-pwa'

export default defineConfig({
  plugins: [
    react(),
    VitePWA({
      registerType: 'autoUpdate',
      // Inject the build revision into the service worker so each deploy busts the cache
      injectRegister: 'auto',
      workbox: {
        // Cache JS/CSS assets aggressively (content-hashed)
        globPatterns: ['**/*.{js,css,html,ico,png,svg,woff2}'],
        // Network-first for API calls — never cache
        runtimeCaching: [
          {
            urlPattern: /^\/api\//,
            handler: 'NetworkOnly',
          },
          {
            urlPattern: /^\/manifest\.webmanifest$/,
            handler: 'NetworkFirst',
            options: {
              cacheName: 'manifest-cache',
              expiration: { maxAgeSeconds: 0 }, // no caching for manifest
            },
          },
        ],
        // This generates a unique revision per build — busts cache on deploy
        navigateFallback: '/index.html',
        navigateFallbackDenylist: [/^\/api/],
      },
      manifest: {
        name: 'FunnelBarn',
        short_name: 'FunnelBarn',
        description: 'Self-hosted funnel analytics. Own your data.',
        theme_color: '#0f1117',
        background_color: '#0f1117',
        display: 'standalone',
        orientation: 'portrait-primary',
        start_url: '/',
        scope: '/',
        categories: ['productivity', 'business'],
        icons: [
          {
            src: '/icons/icon-192.png',
            sizes: '192x192',
            type: 'image/png',
          },
          {
            src: '/icons/icon-512.png',
            sizes: '512x512',
            type: 'image/png',
          },
          {
            src: '/icons/icon-maskable.png',
            sizes: '512x512',
            type: 'image/png',
            purpose: 'maskable',
          },
          {
            src: '/icons/apple-touch-icon.png',
            sizes: '180x180',
            type: 'image/png',
          },
        ],
        shortcuts: [
          {
            name: 'Dashboard',
            url: '/dashboard',
            icons: [{ src: '/icons/icon-192.png', sizes: '192x192' }],
          },
          {
            name: 'Funnels',
            url: '/funnels',
            icons: [{ src: '/icons/icon-192.png', sizes: '192x192' }],
          },
        ],
      },
    }),
  ],
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/test-setup.ts'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'lcov'],
      thresholds: {
        lines: 70,
      },
    },
  },
  build: {
    sourcemap: true,
  },
  server: {
    proxy: {
      '/api': {
        target: process.env.FUNNELBARN_API_URL || 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
