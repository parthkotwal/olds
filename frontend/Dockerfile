# Frontend Dockerfile — two-stage production build.
#
# Stage 1 (builder): installs deps and runs `npm run build`, producing a
# static /dist folder. Vite bakes VITE_* env vars into the bundle at build
# time, so they must be passed as build args here.
#
# Stage 2 (runtime): serves the static files with nginx. The Alpine nginx
# image is ~40MB — far smaller than keeping Node around just to run files.
# nginx also handles gzip compression, cache headers, and SPA routing
# (try_files) without any extra configuration.

# ── Stage 1: build ────────────────────────────────────────────────────────────
FROM node:20-alpine AS builder

WORKDIR /app

# Copy package files first so the npm install layer is cached independently
# from source changes. If package.json hasn't changed, this layer is reused.
COPY package.json package-lock.json* ./
RUN npm ci

# Pass VITE_* vars as build args. Vite replaces import.meta.env.VITE_* at
# build time — the values are embedded in the compiled JS bundle.
# Railway sets these as environment variables; they are forwarded here via ARG.
ARG VITE_API_BASE_URL
ARG VITE_SUPABASE_URL
ARG VITE_SUPABASE_ANON_KEY

# Copy source and build. Output lands in /app/dist.
COPY . .
RUN npm run build

# ── Stage 2: serve ────────────────────────────────────────────────────────────
FROM nginx:alpine

# Copy the built static files into nginx's web root.
COPY --from=builder /app/dist /usr/share/nginx/html

# SPA routing: any path that doesn't match a real file (e.g. /some/route on
# refresh) should return index.html so React Router can handle it client-side.
# We write a minimal nginx config that adds this try_files rule.
RUN printf 'server {\n\
  listen 80;\n\
  root /usr/share/nginx/html;\n\
  index index.html;\n\
  location / {\n\
    try_files $uri $uri/ /index.html;\n\
  }\n\
  # Cache static assets aggressively; index.html must not be cached so\n\
  # users always get the latest JS bundle hash on redeploy.\n\
  location ~* \\.(js|css|png|jpg|jpeg|gif|ico|svg|woff2?)$ {\n\
    expires 1y;\n\
    add_header Cache-Control "public, immutable";\n\
  }\n\
}\n' > /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
