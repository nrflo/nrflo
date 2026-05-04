package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"be/internal/model"
)

func TestListAuditLog_Admin(t *testing.T) {
	as := newAuthServer(t)
	mustLogin(t, as, adminEmail, adminPass)

	// Create 2 users via API to generate "user_create" audit entries.
	r1 := postJSON(t, as.client, as.baseURL+"/api/v1/users",
		`{"email":"al1@test.com","display_name":"AL1","password":"pass12345","role":"viewer"}`)
	drain(r1)

	r2 := postJSON(t, as.client, as.baseURL+"/api/v1/users",
		`{"email":"al2@test.com","display_name":"AL2","password":"pass12345","role":"viewer"}`)
	var u2body struct {
		User struct{ ID string `json:"id"` } `json:"user"`
	}
	json.NewDecoder(r2.Body).Decode(&u2body)
	drain(r2)

	// Reset al2's password → "password_reset_by_admin" audit entry.
	r3 := postJSON(t, as.client, as.baseURL+"/api/v1/users/"+u2body.User.ID+"/reset-password",
		`{"new_password":"newpass123"}`)
	drain(r3)

	resp, err := as.client.Get(as.baseURL + "/api/v1/audit-log")
	if err != nil {
		t.Fatalf("GET /audit-log: %v", err)
	}
	defer drain(resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var out struct {
		Items   []interface{} `json:"items"`
		Total   int           `json:"total"`
		Page    int           `json:"page"`
		PerPage int           `json:"per_page"`
	}
	json.NewDecoder(resp.Body).Decode(&out)

	if out.Total < 3 {
		t.Errorf("total = %d, want >= 3 (2 creates + 1 reset)", out.Total)
	}
	if len(out.Items) == 0 {
		t.Error("items must not be empty")
	}
	if out.Page != 1 {
		t.Errorf("page = %d, want 1", out.Page)
	}
	if out.PerPage != 50 {
		t.Errorf("per_page = %d, want 50 (default)", out.PerPage)
	}
}

func TestListAuditLog_FilterByAction(t *testing.T) {
	as := newAuthServer(t)
	mustLogin(t, as, adminEmail, adminPass)

	// Two user_create entries.
	r1 := postJSON(t, as.client, as.baseURL+"/api/v1/users",
		`{"email":"fa1@test.com","display_name":"FA1","password":"pass12345","role":"viewer"}`)
	drain(r1)
	r2 := postJSON(t, as.client, as.baseURL+"/api/v1/users",
		`{"email":"fa2@test.com","display_name":"FA2","password":"pass12345","role":"viewer"}`)
	var u2body struct {
		User struct{ ID string `json:"id"` } `json:"user"`
	}
	json.NewDecoder(r2.Body).Decode(&u2body)
	drain(r2)

	// One password_reset_by_admin entry.
	r3 := postJSON(t, as.client, as.baseURL+"/api/v1/users/"+u2body.User.ID+"/reset-password",
		`{"new_password":"newpass123"}`)
	drain(r3)

	// Unfiltered total (should be 3).
	respAll, err := as.client.Get(as.baseURL + "/api/v1/audit-log")
	if err != nil {
		t.Fatalf("GET /audit-log: %v", err)
	}
	var allOut struct{ Total int `json:"total"` }
	json.NewDecoder(respAll.Body).Decode(&allOut)
	drain(respAll)

	// Filtered by action=user_create (should be 2).
	respFilt, err := as.client.Get(as.baseURL + "/api/v1/audit-log?action=user_create")
	if err != nil {
		t.Fatalf("GET /audit-log?action=user_create: %v", err)
	}
	defer drain(respFilt)
	var filtOut struct{ Total int `json:"total"` }
	json.NewDecoder(respFilt.Body).Decode(&filtOut)

	if filtOut.Total >= allOut.Total {
		t.Errorf("filtered total (%d) must be less than unfiltered (%d)", filtOut.Total, allOut.Total)
	}
	if filtOut.Total != 2 {
		t.Errorf("user_create total = %d, want 2", filtOut.Total)
	}
}

func TestListAuditLog_Viewer(t *testing.T) {
	as := newAuthServer(t)
	seedUser(t, as.pool, "viewer@audit.com", "pass12345", model.UserRoleViewer, false)
	cl := newJarClient()
	loginAs(t, cl, as.baseURL, "viewer@audit.com", "pass12345")

	resp, err := cl.Get(as.baseURL + "/api/v1/audit-log")
	if err != nil {
		t.Fatalf("GET /audit-log: %v", err)
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("viewer: status = %d, want 403", resp.StatusCode)
	}
}

func TestListAuditLog_Pagination(t *testing.T) {
	as := newAuthServer(t)
	mustLogin(t, as, adminEmail, adminPass)

	// Create 3 users to generate 3 audit entries.
	for _, email := range []string{"pg1@test.com", "pg2@test.com", "pg3@test.com"} {
		r := postJSON(t, as.client, as.baseURL+"/api/v1/users",
			`{"email":"`+email+`","display_name":"PG","password":"pass12345","role":"viewer"}`)
		drain(r)
	}

	// per_page=1 should return exactly 1 item while total reflects all entries.
	resp, err := as.client.Get(as.baseURL + "/api/v1/audit-log?per_page=1")
	if err != nil {
		t.Fatalf("GET /audit-log?per_page=1: %v", err)
	}
	defer drain(resp)

	var out struct {
		Items   []interface{} `json:"items"`
		Total   int           `json:"total"`
		PerPage int           `json:"per_page"`
	}
	json.NewDecoder(resp.Body).Decode(&out)

	if len(out.Items) != 1 {
		t.Errorf("per_page=1: got %d items, want 1", len(out.Items))
	}
	if out.Total < 3 {
		t.Errorf("total = %d, want >= 3", out.Total)
	}
	if out.PerPage != 1 {
		t.Errorf("per_page = %d, want 1", out.PerPage)
	}
}
