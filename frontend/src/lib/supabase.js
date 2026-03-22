import { createClient } from '@supabase/supabase-js'

// These variables are injected at build time by Vite from the environment.
// VITE_ prefix is required — unprefixed variables are never exposed to the browser bundle.
// Values come from docker-compose.yml (local) or Railway environment variables (production).
const supabaseUrl = import.meta.env.VITE_SUPABASE_URL
const supabaseAnonKey = import.meta.env.VITE_SUPABASE_ANON_KEY

if (!supabaseUrl || !supabaseAnonKey) {
  throw new Error(
    'Missing Supabase config: set VITE_SUPABASE_URL and VITE_SUPABASE_ANON_KEY in your environment.'
  )
}

// A single shared Supabase client for the whole app.
// createClient handles session storage (localStorage), token refresh, and
// the OAuth redirect callback automatically — we just call auth methods on it.
export const supabase = createClient(supabaseUrl, supabaseAnonKey)
