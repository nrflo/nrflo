package api

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

var emailRE = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

type createUserReq struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
	Role        string `json:"role"`
}

type updateUserReq struct {
	DisplayName *string `json:"display_name"`
	Role        *string `json:"role"`
	Status      *string `json:"status"`
}

type resetPasswordReq struct {
	NewPassword string `json:"new_password"`
}

// handleListUsers handles GET /api/v1/users.
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := repo.NewUserRepo(s.pool, s.clock).List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if users == nil {
		users = []*model.User{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"users": users})
}

// handleCreateUser handles POST /api/v1/users.
func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var body createUserReq
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	body.Email = strings.ToLower(strings.TrimSpace(body.Email))
	body.DisplayName = strings.TrimSpace(body.DisplayName)

	if body.Email == "" || !emailRE.MatchString(body.Email) {
		writeError(w, http.StatusBadRequest, "valid email is required")
		return
	}
	if body.DisplayName == "" {
		writeError(w, http.StatusBadRequest, "display_name is required")
		return
	}
	if len(body.Password) < 8 || len(body.Password) > 128 {
		writeError(w, http.StatusBadRequest, "password must be 8-128 characters")
		return
	}

	role := model.UserRole(body.Role)
	if role != model.UserRoleAdmin && role != model.UserRoleViewer {
		writeError(w, http.StatusBadRequest, "role must be admin or viewer")
		return
	}

	actingID := getUserID(r)
	id := s.userSvc.GenerateID()

	u, err := s.userSvc.Create(actingID, id, body.Email, body.DisplayName, body.Password, role)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "email_exists")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	appendAudit(s, r, "user_create", "user", u.ID,
		fmt.Sprintf(`{"email":%q}`, u.Email))

	writeJSON(w, http.StatusCreated, map[string]interface{}{"user": u})
}

// handleUpdateUser handles PATCH /api/v1/users/{id}.
func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")

	var body updateUserReq
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	userRepo := repo.NewUserRepo(s.pool, s.clock)
	existing, err := userRepo.Get(targetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	// Merge partial updates onto existing values.
	displayName := existing.DisplayName
	if body.DisplayName != nil {
		displayName = strings.TrimSpace(*body.DisplayName)
	}
	role := existing.Role
	if body.Role != nil {
		role = model.UserRole(*body.Role)
		if role != model.UserRoleAdmin && role != model.UserRoleViewer {
			writeError(w, http.StatusBadRequest, "role must be admin or viewer")
			return
		}
	}
	status := existing.Status
	if body.Status != nil {
		status = model.UserStatus(*body.Status)
		if status != model.UserStatusActive && status != model.UserStatusDisabled {
			writeError(w, http.StatusBadRequest, "status must be active or disabled")
			return
		}
	}

	actingID := getUserID(r)
	if err := s.userSvc.Update(actingID, targetID, displayName, role, status); err != nil {
		if err == service.ErrLastAdmin {
			writeError(w, http.StatusBadRequest, "last_admin")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	updated, err := userRepo.Get(targetID)
	if err != nil || updated == nil {
		writeError(w, http.StatusInternalServerError, "failed to reload user")
		return
	}

	appendAudit(s, r, "user_update", "user", targetID,
		fmt.Sprintf(`{"display_name":%q,"role":%q,"status":%q}`, displayName, role, status))

	writeJSON(w, http.StatusOK, map[string]interface{}{"user": updated})
}

// handleResetUserPassword handles POST /api/v1/users/{id}/reset-password.
func (s *Server) handleResetUserPassword(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")

	var body resetPasswordReq
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.NewPassword) < 8 || len(body.NewPassword) > 128 {
		writeError(w, http.StatusBadRequest, "password must be 8-128 characters")
		return
	}

	actingID := getUserID(r)
	if err := s.userSvc.ResetPassword(actingID, targetID, body.NewPassword); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	appendAudit(s, r, "password_reset_by_admin", "user", targetID, "{}")

	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteUser handles DELETE /api/v1/users/{id}.
func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	actingID := getUserID(r)

	if err := s.userSvc.Delete(actingID, targetID); err != nil {
		switch err {
		case service.ErrSelfDelete:
			writeError(w, http.StatusBadRequest, "cannot_delete_self")
		case service.ErrSystemUser:
			writeError(w, http.StatusBadRequest, "system_user")
		case service.ErrLastAdmin:
			writeError(w, http.StatusBadRequest, "last_admin")
		default:
			if strings.Contains(err.Error(), "not found") {
				writeError(w, http.StatusNotFound, "user not found")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	appendAudit(s, r, "user_delete", "user", targetID, "{}")

	w.WriteHeader(http.StatusNoContent)
}

// appendAudit writes an audit entry, ignoring errors (same pattern as auth service).
func appendAudit(s *Server, r *http.Request, action, resourceType, resourceID, metadata string) {
	_ = repo.NewAuditRepo(s.pool, s.clock).Append(&model.AuditEntry{
		ID:           uuid.New().String(),
		UserID:       getUserID(r),
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		IP:           r.RemoteAddr,
		UserAgent:    r.UserAgent(),
		Metadata:     metadata,
	})
}
