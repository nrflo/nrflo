package api

import (
	"context"
	"net/http"
	"strings"

	"be/internal/auth"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

const userKey contextKey = "user"
const agentSessionKey contextKey = "agent_session"
const servicePrincipalKey contextKey = "service_principal"

// ServicePrincipal represents a request authenticated by a long-lived service
// token. The token grants project-scoped access to the API; principals never
// satisfy `requireAdmin` (which is reserved for human admin users) but may
// satisfy `requireProjectAdmin` when the project_id matches the route.
type ServicePrincipal struct {
	TokenID   string
	ProjectID string
}

// requireAuth ensures the request has a valid, active session.
// Accepts either an SCS session cookie (operator/UI) or an Authorization:
// Bearer <agent_token> header (spawned agent processes; token is minted by the
// spawner and valid only while the agent_sessions row is running/user_interactive).
// Returns 401 on failure. If sessionMgr is nil (test environments) the cookie
// path passes through; the bearer path still works because it doesn't depend
// on the session manager.
// For WebSocket/PTY endpoints that cannot set Authorization headers, use
// requireAuthWith(true, next) to also accept ?token=<bearer> query parameter.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return s.requireAuthWith(false, next)
}

// requireAuthWith is the parameterized form of requireAuth. When acceptQueryToken
// is true, a bearer token may also be supplied via the ?token= query parameter
// as a fallback — this is opt-in and used only for WS/PTY upgrade endpoints
// where browsers cannot set Authorization headers on WebSocket constructors.
func (s *Server) requireAuthWith(acceptQueryToken bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Bearer-token path. Two kinds of tokens are accepted:
		//   1. service tokens — long-lived, admin-minted, project-scoped
		//   2. agent_sessions.spawn_token — short-lived, valid while the
		//      agent_session row is running/user_interactive
		// X-Project, if present, must match the token's project_id.
		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" && acceptQueryToken {
			token = r.URL.Query().Get("token")
		}
		if token != "" {
			svcTok := service.NewServiceTokenService(s.pool, s.clock)
			if t, _ := svcTok.LookupByPlaintext(token); t != nil {
				if hp := r.Header.Get("X-Project"); hp != "" && !strings.EqualFold(hp, t.ProjectID) {
					writeError(w, http.StatusForbidden, "service token project mismatch")
					return
				}
				ctx := context.WithValue(r.Context(), servicePrincipalKey, &ServicePrincipal{
					TokenID:   t.ID,
					ProjectID: t.ProjectID,
				})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

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

// getServicePrincipal retrieves the service-token principal for a bearer-token
// authenticated request, or nil for cookie/agent-token requests.
func getServicePrincipal(r *http.Request) *ServicePrincipal {
	p, _ := r.Context().Value(servicePrincipalKey).(*ServicePrincipal)
	return p
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

// requireProjectAdmin allows either an admin user OR a service principal whose
// project_id matches the route's project context. Project context is resolved
// from the {id} path parameter when present, otherwise from the X-Project
// header. Non-admin human users always get 403. Used for project-scoped
// administrative endpoints (env-vars writes, scheduled tasks, python scripts).
func (s *Server) requireProjectAdmin(next http.Handler) http.Handler {
	return s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := getUser(r); u != nil && u.Role == model.UserRoleAdmin {
			next.ServeHTTP(w, r)
			return
		}
		if sp := getServicePrincipal(r); sp != nil {
			scope := r.PathValue("id")
			if scope == "" {
				scope = r.Header.Get("X-Project")
			}
			if scope != "" && strings.EqualFold(scope, sp.ProjectID) {
				next.ServeHTTP(w, r.WithContext(r.Context()))
				return
			}
			writeError(w, http.StatusForbidden, "project scope required")
			return
		}
		writeError(w, http.StatusForbidden, "admin access required")
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
