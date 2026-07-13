# syntax=docker/dockerfile:1.7
#
# Linux multi-arch image for nrflo_server.
# Ships with api-mode off by default; admin enables it via Settings UI.
# Bundles the native Claude Code CLI (Bun-compiled musl binary, no Node) and
# the native codex CLI (rust musl binary, no Node) so cli-mode works once
# provider credentials are supplied; opencode is still absent. api-mode
# remains the zero-config default. poppler-utils (pdftotext/pdftoppm) backs
# the codex read_document hybrid path for PDFs.

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
ARG CLAUDE_VERSION=2.1.207
# Bundled codex CLI version (native rust musl binary, no Node). Codex publishes
# no checksum manifest (only sigstore bundles), so the per-arch tarball sha256
# is pinned here; bump both together.
ARG CODEX_VERSION=0.144.3
ARG CODEX_SHA256_AMD64=b9b4ae8e9b561c64dfbc5ef52c6319cba750ac87de3c7f55885026231e3aea89
ARG CODEX_SHA256_ARM64=dd76cfd5a2cf9bcf0e3224afe28e23065cfd27262e06e0ffbc8fa40343f0905a

# Runtime deps: git + tini + python, the musl libs and ripgrep the native
# Claude binary needs (its bundled ripgrep is glibc-built, unusable on musl),
# and poppler-utils so codex agents can pdftotext/pdftoppm PDFs handed to them
# by the read_document hybrid tool (codex has no native PDF ingestion).
RUN apk add --no-cache python3 py3-pip ca-certificates git tini \
      libgcc libstdc++ ripgrep poppler-utils \
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

# Native codex CLI: rust musl release tarball, sha256-pinned per arch (see the
# CODEX_* args above), installed root-owned at /usr/local/bin/codex. The build
# smoke-tests `codex --version` so a binary that can't exec on musl fails here.
RUN set -eux; \
    apk add --no-cache --virtual .codex-build curl; \
    case "$TARGETARCH" in \
      amd64) triple="x86_64-unknown-linux-musl";  sha="$CODEX_SHA256_AMD64" ;; \
      arm64) triple="aarch64-unknown-linux-musl"; sha="$CODEX_SHA256_ARM64" ;; \
      *) echo "unsupported TARGETARCH=$TARGETARCH" >&2; exit 1 ;; \
    esac; \
    curl -fsSL -o /tmp/codex.tar.gz \
      "https://github.com/openai/codex/releases/download/rust-v${CODEX_VERSION}/codex-${triple}.tar.gz"; \
    echo "$sha  /tmp/codex.tar.gz" | sha256sum -c -; \
    tar -xzf /tmp/codex.tar.gz -C /tmp "codex-${triple}"; \
    mv "/tmp/codex-${triple}" /usr/local/bin/codex; \
    chmod 0755 /usr/local/bin/codex; \
    rm /tmp/codex.tar.gz; \
    /usr/local/bin/codex --version; \
    apk del .codex-build

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

# api-mode ships off by default (enable via Settings UI); cli-mode can drive
# the bundled claude (ANTHROPIC_API_KEY / ANTHROPIC_OAUTH_TOKEN) or codex
# (OPENAI_API_KEY / CODEX_API_KEY auth) once credentials are in the env.
ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/nrflo_server", "serve", \
            "--host", "0.0.0.0", "--port", "6587"]
