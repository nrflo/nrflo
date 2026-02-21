# usagelimits Package

## Overview

Fetches CLI tool usage limits (Claude, Codex) via PTY scraping and caches results in-memory with optional DB persistence.

## Persistence Flow

```
Server Start
  → NewServer(): create db.Pool + PreferencesService, pass to Cache as Store
  → startUsageLimitsFetcher():
      1. cache.LoadFromDB()
         → DB has fresh data (<30min)? → populate cache immediately
         → DB stale/empty? → fall through
      2. If no DB data: fetch() via PTY (~15s)
      3. Start 20-min ticker for periodic refresh
  → On each fetch():
      → FetchAll() via PTY scraping
      → cache.Set(data) → updates memory + writes to DB asynchronously
```

## Key Types

- **`Store` interface**: `{ Get(name) (*model.Preference, error); Set(name, value) error }` — implemented by `PreferencesService`, keeps package decoupled from service layer
- **`Cache`**: Thread-safe in-memory cache with optional `Store` for DB persistence
- **`UsageLimits`**: Holds Claude + Codex usage data + `FetchedAt` timestamp

## Preference Key

Stored under `"usage_limits"` in the `preferences` table. Value is JSON-serialized `UsageLimits` struct.

## Staleness Threshold

30 minutes (`stalenessThreshold`). Uses `pref.UpdatedAt` (DB write time), not `FetchedAt` (scrape time).

## Files

| File | Purpose |
|------|---------|
| `cache.go` | Cache struct, Store interface, Get/Set/LoadFromDB |
| `types.go` | UsageLimits, ToolUsage, UsageMetric types |
| `fetch.go` | PTY-based fetching (FetchAll, FetchClaude, FetchCodex) |
| `parse.go` | Output parsing for claude/codex CLI tools |
| `ansi.go` | ANSI escape sequence stripping |
