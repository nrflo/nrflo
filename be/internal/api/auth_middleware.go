package api

import (
	"context"
	"net/http"
	"strings"

	"be/internal/auth"
	"be/internal/model"
	"be/internal/repo"
)

const userKey contextKey = "user"
const agentSessionKey contextKey = "agent_session"

// requireAuth ensures the request has a valid, active session.
// Accepts either an SCS session cookie (operator/UI) or an Authorization:
// Bearer <agent_token> header (spawned agent processes; token is minted by the
// spawner and valid only while the agent_sessions row is running/user_interactive).
// Returns 401 on failure. If sessionMgr is nil (test environments) the cookie
// path passes through; the bearer path still works because it doesn't depend
// on the session manager.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Bearer-token path (spawned agents). Tokens are short-lived and
		// scoped to a single agent_session row; we treat them as project-
		// scoped credentials and reject any X-Project mismatch.
		if token := bearerToken(r.Header.Get("Authorization")); token != "" {
			sessRepo := repo.NewAgentSessionRepo(s.pool, s.clock)
			sess, err := sessRepo.GetByToken(token)
			if err == nil && sess != nil {
				if hp := r.Header.Get("X-Project"); hp != "" && !strings.EqualFold(hp, sess.ProjectID) {
					writeError(w, http.StatusForbidden, "agent token project mismatch")
					return
				}
				ctx := context.WithValue(r.Context(), agentSessionKey, sess)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			// Unknown / expired token — fall through to cookie path so a
			// browser request with both a stale token and a valid cookie
			// still works. Almost always the cookie path will also reject.
		}

		if s.sessionMgr == nil {
			next.ServeHTTP(w, r)
			return
		}
		uid := auth.UserID(r.Context(), s.sessionMgr)
		if uid == "" {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		userRepo := repo.NewUserRepo(s.pool, s.clock)
		u, err := userRepo.Get(uid)
		if err != nil || u == nil || u.Status == model.UserStatusDisabled {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		ctx := context.WithValue(r.Context(), userKey, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// bearerToken extracts the token from an "Authorization: Bearer <token>"
// header value, or returns "" if the header is empty / not Bearer.
func bearerToken(h string) string {
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}

// getAgentSession retrieves the agent session associated with a bearer-token
// authenticated request, or nil for cookie-authenticated (operator) requests.
func getAgentSession(r *http.Request) *model.AgentSession {
	s, _ := r.Context().Value(agentSessionKey).(*model.AgentSession)
	return s
}

// requireAdmin composes requireAuth and returns 403 if the user is not an admin.
func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := getUser(r)
		if u == nil || u.Role != model.UserRoleAdmin {
			writeError(w, http.StatusForbidden, "admin access required")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// getUser retrieves the authenticated user from the request context.
func getUser(r *http.Request) *model.User {
	u, _ := r.Context().Value(userKey).(*model.User)
	return u
}

// getUserID retrieves the authenticated user's ID from the request context.
func getUserID(r *http.Request) string {
	if u := getUser(r); u != nil {
		return u.ID
	}
	return ""
}
