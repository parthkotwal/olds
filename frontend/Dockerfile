# Frontend Dockerfile — two-stage production build.
#
# Stage 1 (builder): installs deps and runs `npm run build`, producing a
# static /dist folder. Vite bakes VITE_* env vars into the bundle at build
# time, so they must be passed as build args here.
#
# Stage 2 (runtime): serves the static files with nginx. The Alpine nginx
# image is ~40MB — far smaller than keeping Node around just to run files.
#
# PORT handling: Railway assigns a port via the PORT env var at runtime.
# The official nginx:alpine image processes files in /etc/nginx/templates/
# with envsubst on startup — nginx.conf.template uses ${PORT} and the
# entrypoint substitutes the real value before nginx starts.

# ── Stage 1: build ────────────────────────────────────────────────────────────
FROM node:20-alpine AS builder

WORKDIR /app

COPY package.json package-lock.json* ./
RUN npm ci

# VITE_* vars are baked into the JS bundle at build time by Vite.
# They must arrive as build args — runtime environment vars are ignored.
ARG VITE_API_BASE_URL
ARG VITE_SUPABASE_URL
ARG VITE_SUPABASE_ANON_KEY

COPY . .
RUN npm run build

# ── Stage 2: serve ────────────────────────────────────────────────────────────
FROM nginx:alpine

# Copy the compiled static files into nginx's web root.
COPY --from=builder /app/dist /usr/share/nginx/html

# The nginx:alpine image's entrypoint runs envsubst on every *.template file
# in /etc/nginx/templates/ and writes the result to /etc/nginx/conf.d/.
# This is how we pass the Railway-assigned $PORT to nginx at container startup.
COPY nginx.conf.template /etc/nginx/templates/default.conf.template

# Remove the default nginx config so our template is the only one loaded.
RUN rm -f /etc/nginx/conf.d/default.conf

# EXPOSE is documentation only — Railway ignores it and uses $PORT instead.
EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
