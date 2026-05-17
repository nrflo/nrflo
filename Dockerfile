# syntax=docker/dockerfile:1.7
#
# Linux multi-arch image for nrflo_server.
# Ships with api-mode off by default; admin enables it via Settings UI.
# Ships with git but no agent CLIs (no claude/codex/opencode) — cli-mode
# would have nothing to spawn, so api-mode is the practical execution backend.

# ---------------------------------------------------------------------------
# Stage 1 — UI build (Node, host-arch, runs once for both target arches)
# ---------------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM node:22-alpine AS ui-builder
WORKDIR /src/ui
COPY ui/package.json ui/package-lock.json ./
RUN npm ci
COPY ui/ ./
RUN npm run build

# ---------------------------------------------------------------------------
# Stage 2 — Go cross-compile (host-arch builder, target-arch output)
# ---------------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS go-builder
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src

# Module cache layer
COPY be/go.mod be/go.sum ./be/
RUN cd be && go mod download

# Source + embed inputs (matches Makefile build-ui + embed-assets targets)
COPY be/ ./be/
COPY agent_manual.md ./agent_manual.md
COPY --from=ui-builder /src/ui/dist ./be/internal/static/dist
RUN cp agent_manual.md be/internal/static/agent_manual.md

# Pure-static build: no CGO, no `tray` tag (uses serve_notray.go).
# `creack/pty` is pure-Go on Linux; modernc.org/sqlite is pure-Go too.
RUN cd be && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath \
      -ldflags="-s -w -X be/internal/cli.version=${VERSION}" \
      -o /out/nrflo_server ./cmd/server

# ---------------------------------------------------------------------------
# Stage 3 — runtime
# ---------------------------------------------------------------------------
FROM alpine:3.20 AS runtime

RUN apk add --no-cache python3 py3-pip ca-certificates git tini \
 && addgroup -S nrflo \
 && adduser -S -G nrflo -u 65532 -h /data nrflo \
 && mkdir -p /data \
 && chown nrflo:nrflo /data

COPY --from=go-builder /out/nrflo_server /usr/local/bin/nrflo_server

ENV NRFLO_HOME=/data
VOLUME ["/data"]
EXPOSE 6587
USER nrflo:nrflo
WORKDIR /data

# The image ships no agent CLIs; enable api-mode via Settings UI after deploy.
ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/nrflo_server", "serve", \
            "--host", "0.0.0.0", "--port", "6587"]
