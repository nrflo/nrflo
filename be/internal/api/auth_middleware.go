package api

import (
	"context"
	"net/http"

	"be/internal/auth"
	"be/internal/model"
	"be/internal/repo"
)

const userKey contextKey = "user"

// requireAuth ensures the request has a valid, active session.
// Loads the user and stashes it in context. Returns 401 on failure.
// If sessionMgr is nil (test environments), passes through without auth.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
