package auth

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/alexedwards/scs/sqlite3store"
	"github.com/alexedwards/scs/v2"
)

const sessionCookieName = "nrflo_session"
const sessionUserIDKey = "userID"

// NewManager constructs an SCS SessionManager backed by sqlite3store.
// dev=true disables the Secure cookie flag (for local HTTP development).
func NewManager(database *sql.DB, dev bool) *scs.SessionManager {
	sm := scs.New()
	sm.Cookie.Name = sessionCookieName
	sm.Cookie.HttpOnly = true
	sm.Cookie.Secure = !dev
	sm.Cookie.SameSite = http.SameSiteStrictMode
	sm.Lifetime = 24 * time.Hour
	sm.IdleTimeout = 8 * time.Hour
	sm.Store = sqlite3store.New(database)
	return sm
}

// PutUserID stores a user ID in the session.
func PutUserID(ctx context.Context, sm *scs.SessionManager, userID string) {
	sm.Put(ctx, sessionUserIDKey, userID)
}

// UserID retrieves the user ID from the session. Returns "" if not set.
func UserID(ctx context.Context, sm *scs.SessionManager) string {
	v, _ := sm.Get(ctx, sessionUserIDKey).(string)
	return v
}

// Renew renews the session token to prevent fixation attacks.
func Renew(ctx context.Context, sm *scs.SessionManager) error {
	return sm.RenewToken(ctx)
}
