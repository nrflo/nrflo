# be/internal/auth

Auth foundation: Argon2id password hashing and SCS session management. No HTTP handlers here — wiring lives in the API layer (T2).

## Files

| File | Purpose |
|------|---------|
| `password.go` | Argon2id Hash/Verify with PHC encoding |
| `session.go` | SCS SessionManager constructor + helpers |
| `seedhash/main.go` | One-shot tool to generate the seeded admin hash |

## Argon2id Parameters

| Parameter | Value |
|-----------|-------|
| Memory (m) | 65536 KiB (64 MiB) |
| Iterations (t) | 3 |
| Parallelism (p) | 2 |
| Salt length | 16 bytes |
| Key length | 32 bytes |

PHC format: `$argon2id$v=19$m=65536,t=3,p=2$<salt-b64>$<key-b64>` (base64 raw std encoding, no padding).

`Verify` uses `subtle.ConstantTimeCompare` to prevent timing attacks. Returns `ErrHashMismatch` on mismatch, `ErrMalformedHash` on invalid PHC string.

## SCS Session Manager

`NewManager(database *sql.DB, dev bool)` returns a `*scs.SessionManager` with:

- Cookie name: `nrflo_session`
- HttpOnly: `true`
- Secure: `!dev` (T2 passes `dev=true` when binding to localhost)
- SameSite: `Strict`
- Lifetime: `24h`
- IdleTimeout: `8h`
- Store: `sqlite3store.New(database)` — sessions persisted in the `sessions` table (migration 000076)

Helpers `PutUserID`, `UserID`, `Renew` wrap the session manager calls to avoid string-key typos at call sites.

## Migration Notes

Migrations 000075–000078 are picked up automatically by the `//go:embed *.sql` directive in `be/internal/db/migrations/embed.go`. The integration test harness in `be/internal/integration/testmain_test.go` calls `db.NewPoolPath` which runs `RunMigrations`, so the new tables are included in the template DB on first test run — no harness change required.

The seeded admin row (migration 000078) uses `INSERT OR IGNORE` — safe to re-run. Default credentials: `admin` / `admin`, `must_change_password=0`.
