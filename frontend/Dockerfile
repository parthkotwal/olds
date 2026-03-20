# Frontend Dockerfile — Vite dev server for local development.
#
# For production you would build (npm run build) and serve the static output
# with nginx. For this project we use the Vite dev server directly, which
# supports hot module replacement — useful during active development.
#
# node:20-alpine is a small Alpine Linux base with Node.js 20 LTS (~180MB).
# The Alpine variant is preferred over the full Debian image (~900MB) for
# containers that just need to run Node. It lacks some system libs but
# everything Vite + React needs is available.

FROM node:20-alpine

WORKDIR /app

# Copy package files before source code — same caching strategy as other
# services: if package.json hasn't changed, npm install is skipped on rebuild.
COPY package.json package-lock.json* ./

# Use npm install rather than npm ci so the Dockerfile works even without a
# committed lock file (useful when setting up the project for the first time).
# Once a lock file exists, switch to npm ci for reproducible installs.
RUN npm install

# Copy application source
COPY . .

EXPOSE 5173

# Vite's dev server binds to localhost by default. --host 0.0.0.0 makes it
# reachable from outside the container (necessary for Docker port mapping).
CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0", "--port", "5173"]
