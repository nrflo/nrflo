package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"be/internal/model"
	"be/internal/service"
)

func TestDenyNonAdminGlobalWrite(t *testing.T) {
	cases := []struct {
		name     string
		project  string
		user     *model.User
		wantDeny bool
	}{
		{"non-global, nil user -> allow", "proj1", nil, false},
		{"non-global, viewer -> allow", "proj1", &model.User{Role: model.UserRoleViewer}, false},
		{"non-global, admin -> allow", "proj1", &model.User{Role: model.UserRoleAdmin}, false},
		{"global, nil user (e.g. bearer) -> deny", service.GlobalProjectID, nil, true},
		{"global, viewer -> deny", service.GlobalProjectID, &model.User{Role: model.UserRoleViewer}, true},
		{"global, admin -> allow", service.GlobalProjectID, &model.User{Role: model.UserRoleAdmin}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", nil)
			if tc.user != nil {
				r = r.WithContext(context.WithValue(r.Context(), userKey, tc.user))
			}
			w := httptest.NewRecorder()
			got := denyNonAdminGlobalWrite(w, r, tc.project)
			if got != tc.wantDeny {
				t.Fatalf("denyNonAdminGlobalWrite = %v, want %v", got, tc.wantDeny)
			}
			if tc.wantDeny && w.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", w.Code)
			}
		})
	}
}
