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
        // Take control immediately on activation so updates go live without waiting for tab close
        skipWaiting: true,
        clientsClaim: true,
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
      reporter: ['text', 'text-summary', 'json-summary', 'lcov'],
      // Count EVERY source file, not just those a test happens to import, so a
      // new untested file drags the numbers down (it can't hide behind covered
      // files). Tests/setup/type-shims are excluded.
      all: true,
      include: ['src/**/*.{ts,tsx}'],
      exclude: [
        'src/**/*.test.{ts,tsx}',
        'src/test-setup.ts',
        'src/setupTests.ts',
        'src/vite-env.d.ts',
      ],
      // Coverage ratchet floors — PINNED AT CURRENT (whole percent). They may
      // only be raised, never lowered. A drop below any floor fails the build.
      thresholds: {
        lines: 26,
        statements: 23,
        functions: 17,
        branches: 23,
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
