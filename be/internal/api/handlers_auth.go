package api

import (
	"fmt"
	"net/http"

	"be/internal/auth"
	"be/internal/service"
)

// handleAuthLogin processes POST /api/v1/auth/login.
// Public route (no requireAuth), but LoadAndSave is in the handler chain.
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	key := rateLimitKey(r.RemoteAddr, body.Email)
	ok, retryAfter := s.rateLimiter.TryAcquire(key)
	if !ok {
		w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
		writeError(w, http.StatusTooManyRequests, "too many login attempts")
		return
	}

	u, err := s.authSvc.Login(body.Email, body.Password, r.RemoteAddr, r.UserAgent())
	if err != nil {
		if err == service.ErrInvalidCredentials || err == service.ErrUserDisabled {
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}

	if err := auth.Renew(r.Context(), s.sessionMgr); err != nil {
		writeError(w, http.StatusInternalServerError, "session error")
		return
	}
	auth.PutUserID(r.Context(), s.sessionMgr, u.ID)

	writeJSON(w, http.StatusOK, map[string]interface{}{"user": u})
}

// handleAuthLogout processes POST /api/v1/auth/logout.
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	_ = s.sessionMgr.Destroy(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

// handleAuthMe processes GET /api/v1/auth/me.
func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	u := getUser(r)
	writeJSON(w, http.StatusOK, map[string]interface{}{"user": u})
}

// handleAuthChangePassword processes POST /api/v1/auth/change-password.
func (s *Server) handleAuthChangePassword(w http.ResponseWriter, r *http.Request) {
	uid := getUserID(r)
	var body struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.authSvc.ChangePassword(uid, body.Current, body.New, r.RemoteAddr, r.UserAgent()); err != nil {
		if err == service.ErrInvalidCredentials {
			writeError(w, http.StatusBadRequest, "invalid current password")
			return
		}
		writeError(w, http.StatusInternalServerError, "change password failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
