import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],

  server: {
    port: 5173,
    host: true, // needed for Docker (listens on 0.0.0.0, not just localhost)
  },

  build: {
    rollupOptions: {
      output: {
        // Split vendor code into separate chunks so a change to your app code
        // doesn't bust the cache for React or Supabase. On redeploy, browsers
        // only re-download the changed chunk (typically the small app chunk),
        // not the whole bundle.
        manualChunks: {
          'react-vendor': ['react', 'react-dom'],
          'supabase': ['@supabase/supabase-js'],
        },
      },
    },
  },
})
