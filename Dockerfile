# syntax=docker/dockerfile:1.7
#
# Linux multi-arch image for nrflo_server.
# Ships with api-mode off by default; admin enables it via Settings UI.
# Bundles the native Claude Code CLI (Bun-compiled musl binary, no Node) so
# cli-mode works once an ANTHROPIC_API_KEY / ANTHROPIC_OAUTH_TOKEN is supplied;
# codex/opencode are still absent. api-mode remains the zero-config default.

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
COPY doc/ ./doc/
COPY --from=ui-builder /src/ui/dist ./be/internal/static/dist
RUN mkdir -p be/internal/static/doc && cp doc/*.md be/internal/static/doc/

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
ARG TARGETARCH
# Bundled Claude Code version. Pinned for reproducibility; bump deliberately.
# "latest" is also accepted and resolves at build time.
ARG CLAUDE_VERSION=2.1.178

# Runtime deps: git + tini + python, plus the musl libs and ripgrep the native
# Claude binary needs (its bundled ripgrep is glibc-built, unusable on musl).
RUN apk add --no-cache python3 py3-pip ca-certificates git tini \
      libgcc libstdc++ ripgrep \
 && addgroup -S nrflo \
 && adduser -S -G nrflo -u 65532 -h /data nrflo \
 && mkdir -p /data \
 && chown nrflo:nrflo /data

# Native Claude Code CLI: standalone Bun-compiled musl binary (no Node runtime).
# Fetched per target arch, sha256-verified against the signed release manifest,
# installed root-owned at /usr/local/bin (outside the /data volume). The build
# smoke-tests `claude --version` so a binary that can't exec on musl fails here.
RUN set -eux; \
    apk add --no-cache --virtual .claude-build curl jq; \
    case "$TARGETARCH" in \
      amd64) cc_arch="x64" ;; \
      arm64) cc_arch="arm64" ;; \
      *) echo "unsupported TARGETARCH=$TARGETARCH" >&2; exit 1 ;; \
    esac; \
    platform="linux-${cc_arch}-musl"; \
    base="https://downloads.claude.ai/claude-code-releases"; \
    ver="$CLAUDE_VERSION"; \
    if [ "$ver" = "latest" ]; then ver="$(curl -fsSL "$base/latest")"; fi; \
    curl -fsSL -o /usr/local/bin/claude "$base/$ver/$platform/claude"; \
    sha="$(curl -fsSL "$base/$ver/manifest.json" | jq -r --arg p "$platform" '.platforms[$p].checksum')"; \
    echo "$sha  /usr/local/bin/claude" | sha256sum -c -; \
    chmod 0755 /usr/local/bin/claude; \
    DISABLE_AUTOUPDATER=1 USE_BUILTIN_RIPGREP=0 /usr/local/bin/claude --version; \
    apk del .claude-build

COPY --from=go-builder /out/nrflo_server /usr/local/bin/nrflo_server

# USE_BUILTIN_RIPGREP=0 -> use the apk musl ripgrep; DISABLE_AUTOUPDATER=1 ->
# pin the bundled version (the root-owned binary is unwritable to the agent user
# anyway). Both propagate to spawned claude agents via the inherited env.
ENV NRFLO_HOME=/data \
    USE_BUILTIN_RIPGREP=0 \
    DISABLE_AUTOUPDATER=1
VOLUME ["/data"]
EXPOSE 6587
USER nrflo:nrflo
WORKDIR /data

# api-mode ships off by default (enable via Settings UI); cli-mode can drive the
# bundled claude once ANTHROPIC_API_KEY / ANTHROPIC_OAUTH_TOKEN is in the env.
ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/nrflo_server", "serve", \
            "--host", "0.0.0.0", "--port", "6587"]
