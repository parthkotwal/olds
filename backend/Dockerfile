# Multi-stage Docker build for the Olds Go backend.
#
# Stage 1 (builder): compiles the Go binary inside a full Go environment.
# Stage 2 (runtime): copies only the compiled binary into a minimal Alpine image.
#
# This keeps the final image small (~15MB vs ~300MB) because the Go toolchain
# and source code are left behind in the builder stage. This is a standard Go
# Docker pattern — analogous to a "build" step in a Node.js CI pipeline where
# you install devDependencies only during build, not in production.

# ── Stage 1: Build ────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy dependency files first and download modules before copying source code.
# Docker builds in layers — if go.mod/go.sum haven't changed, this layer is
# cached and `go mod download` is skipped on subsequent builds. Only re-runs
# when you add or update a dependency. Same pattern as copying package.json
# before npm install in a Node Dockerfile.
COPY go.mod go.sum* ./
RUN go mod download

# Now copy the rest of the source code and build the binary.
COPY . .

# Build flags explained:
#   CGO_ENABLED=0  — disables C extensions, producing a fully static binary
#                    with no shared library dependencies
#   GOOS=linux     — cross-compile for Linux even if building on macOS
#   -o /server     — output the binary to /server
#   ./cmd/server   — the package path to build (the main package)
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

# ── Stage 2: Runtime ──────────────────────────────────────────────────────────
# Using alpine (not scratch) so we have a shell available for debugging
# during development (docker exec, apk add, etc.).
FROM alpine:3.19

# Copy only the compiled binary from the builder stage — nothing else.
COPY --from=builder /server /server

EXPOSE 8080

CMD ["/server"]
