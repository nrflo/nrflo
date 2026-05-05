package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/service"
)

// Template DB seeded admin credentials (migration 000078).
const (
	adminEmail = "admin"
	adminPass  = "admin"
)

// patchJSON sends a PATCH request with a JSON-marshaled body.
func patchJSON(t *testing.T, cl *http.Client, url string, body interface{}) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("PATCH", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := cl.Do(req)
	if err != nil {
		t.Fatalf("PATCH %s: %v", url, err)
	}
	return resp
}

// deleteReq sends a DELETE request.
func deleteReq(t *testing.T, cl *http.Client, url string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("DELETE", url, nil)
	resp, err := cl.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", url, err)
	}
	return resp
}

// newJarClient returns a new http.Client with its own cookie jar.
func newJarClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

// loginAs logs in using cl and fails the test if the response is not 200.
func loginAs(t *testing.T, cl *http.Client, baseURL, email, pass string) {
	t.Helper()
	resp := loginHTTP(t, cl, baseURL, email, pass)
	defer drain(resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("loginAs %s: status = %d, want 200", email, resp.StatusCode)
	}
}

func TestListUsers_Admin(t *testing.T) {
	as := newAuthServer(t)
	mustLogin(t, as, adminEmail, adminPass)

	resp, err := as.client.Get(as.baseURL + "/api/v1/users")
	if err != nil {
		t.Fatalf("GET /users: %v", err)
	}
	defer drain(resp)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// password_hash is json:"-" on model.User and must never appear in output.
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "password_hash") {
		t.Error("response body must not contain password_hash")
	}

	users, _ := out["users"].([]interface{})
	if len(users) == 0 {
		t.Error("expected non-empty users list")
	}
}

func TestListUsers_Viewer(t *testing.T) {
	as := newAuthServer(t)
	seedUser(t, as.pool, "view@test.com", "pass12345", model.UserRoleViewer, false)
	cl := newJarClient()
	loginAs(t, cl, as.baseURL, "view@test.com", "pass12345")

	resp, err := cl.Get(as.baseURL + "/api/v1/users")
	if err != nil {
		t.Fatalf("GET /users: %v", err)
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("viewer GET /users: status = %d, want 403", resp.StatusCode)
	}
}

func TestCreateUser_Success(t *testing.T) {
	as := newAuthServer(t)
	mustLogin(t, as, adminEmail, adminPass)

	body := `{"email":"new@test.com","display_name":"New User","password":"pass12345","role":"viewer"}`
	resp := postJSON(t, as.client, as.baseURL+"/api/v1/users", body)
	defer drain(resp)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /users: status = %d, want 201", resp.StatusCode)
	}

	var out struct {
		User struct {
			Email              string `json:"email"`
			MustChangePassword bool   `json:"must_change_password"`
		} `json:"user"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	if out.User.Email != "new@test.com" {
		t.Errorf("email = %q, want new@test.com", out.User.Email)
	}
	if !out.User.MustChangePassword {
		t.Error("must_change_password = false, want true for newly created user")
	}
}

func TestCreateUser_ViewerForbidden(t *testing.T) {
	as := newAuthServer(t)
	seedUser(t, as.pool, "viewer2@test.com", "pass12345", model.UserRoleViewer, false)
	cl := newJarClient()
	loginAs(t, cl, as.baseURL, "viewer2@test.com", "pass12345")

	body := `{"email":"x@test.com","display_name":"X","password":"pass12345","role":"viewer"}`
	resp := postJSON(t, cl, as.baseURL+"/api/v1/users", body)
	defer drain(resp)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("viewer POST /users: status = %d, want 403", resp.StatusCode)
	}
}

func TestCreateUser_DuplicateEmail(t *testing.T) {
	as := newAuthServer(t)
	mustLogin(t, as, adminEmail, adminPass)

	body := `{"email":"dup@test.com","display_name":"Dup","password":"pass12345","role":"viewer"}`
	r1 := postJSON(t, as.client, as.baseURL+"/api/v1/users", body)
	drain(r1)

	r2 := postJSON(t, as.client, as.baseURL+"/api/v1/users", body)
	defer drain(r2)
	if r2.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate email: status = %d, want 409", r2.StatusCode)
	}
	var errOut map[string]string
	json.NewDecoder(r2.Body).Decode(&errOut)
	if errOut["error"] != "email_exists" {
		t.Errorf("error = %q, want email_exists", errOut["error"])
	}
}

func TestCreateUser_InvalidInput(t *testing.T) {
	as := newAuthServer(t)
	mustLogin(t, as, adminEmail, adminPass)

	cases := []struct{ name, body string }{
		{"invalid_email", `{"email":"notanemail","display_name":"A","password":"pass12345","role":"viewer"}`},
		{"empty_email", `{"email":"","display_name":"A","password":"pass12345","role":"viewer"}`},
		{"short_password", `{"email":"a@b.com","display_name":"A","password":"abc","role":"viewer"}`},
		{"empty_display_name", `{"email":"a@b.com","display_name":"","password":"pass12345","role":"viewer"}`},
		{"invalid_role", `{"email":"a@b.com","display_name":"A","password":"pass12345","role":"superuser"}`},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			resp := postJSON(t, as.client, as.baseURL+"/api/v1/users", c.body)
			defer drain(resp)
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("%s: status = %d, want 400", c.name, resp.StatusCode)
			}
		})
	}
}

// TestUpdateUser_LastAdmin verifies that demoting the only active admin returns 400 last_admin.
func TestUpdateUser_LastAdmin(t *testing.T) {
	as := newAuthServer(t)
	// Template admin is the only active admin in the freshly copied DB.
	mustLogin(t, as, adminEmail, adminPass)

	meResp, err := as.client.Get(as.baseURL + "/api/v1/auth/me")
	if err != nil {
		t.Fatalf("GET /auth/me: %v", err)
	}
	var meOut struct {
		User struct{ ID string `json:"id"` } `json:"user"`
	}
	json.NewDecoder(meResp.Body).Decode(&meOut)
	drain(meResp)

	resp := patchJSON(t, as.client, as.baseURL+"/api/v1/users/"+meOut.User.ID,
		map[string]string{"role": "viewer"})
	defer drain(resp)

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("demote last admin: status = %d, want 400", resp.StatusCode)
	}
	var errOut map[string]string
	json.NewDecoder(resp.Body).Decode(&errOut)
	if errOut["error"] != "last_admin" {
		t.Errorf("error = %q, want last_admin", errOut["error"])
	}
}

func TestDeleteUser_Self(t *testing.T) {
	as := newAuthServer(t)
	adminID := seedUser(t, as.pool, "admin2@test.com", "pass12345", model.UserRoleAdmin, false)
	cl := newJarClient()
	loginAs(t, cl, as.baseURL, "admin2@test.com", "pass12345")

	resp := deleteReq(t, cl, as.baseURL+"/api/v1/users/"+adminID)
	defer drain(resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("delete self: status = %d, want 400", resp.StatusCode)
	}
	var errOut map[string]string
	json.NewDecoder(resp.Body).Decode(&errOut)
	if errOut["error"] != "cannot_delete_self" {
		t.Errorf("error = %q, want cannot_delete_self", errOut["error"])
	}
}

// TestDeleteUser_LastAdmin_ServiceLayer tests ErrLastAdmin at the service level since
// the HTTP admin gate always requires the acting user to be an active admin, making it
// impossible to have count=1 when the target is a different admin.
func TestDeleteUser_LastAdmin_ServiceLayer(t *testing.T) {
	as := newAuthServer(t)
	svc := service.NewUserService(as.pool, clock.Real())

	adminID := seedUser(t, as.pool, "admin3@test.com", "pass12345", model.UserRoleAdmin, false)
	// Disable the template admin so admin3 is the sole active admin.
	if _, err := as.pool.Exec(`UPDATE users SET status='disabled' WHERE email=?`, adminEmail); err != nil {
		t.Fatalf("disable template admin: %v", err)
	}

	err := svc.Delete("other-user-id", adminID)
	if err != service.ErrLastAdmin {
		t.Errorf("Delete last admin: got %v, want ErrLastAdmin", err)
	}
}

func TestResetPassword_Success(t *testing.T) {
	as := newAuthServer(t)
	targetID := seedUser(t, as.pool, "target@test.com", "oldpass12", model.UserRoleViewer, false)
	mustLogin(t, as, adminEmail, adminPass)

	resp := postJSON(t, as.client, as.baseURL+"/api/v1/users/"+targetID+"/reset-password",
		`{"new_password":"newpass123"}`)
	defer drain(resp)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("reset-password: status = %d, want 204", resp.StatusCode)
	}

	// Login with the new password succeeds.
	cl := newJarClient()
	loginAs(t, cl, as.baseURL, "target@test.com", "newpass123")

	// /auth/me must report must_change_password=true after an admin reset.
	meResp, err := cl.Get(as.baseURL + "/api/v1/auth/me")
	if err != nil {
		t.Fatalf("GET /auth/me: %v", err)
	}
	defer drain(meResp)
	var me struct {
		User struct {
			MustChangePassword bool `json:"must_change_password"`
		} `json:"user"`
	}
	json.NewDecoder(meResp.Body).Decode(&me)
	if !me.User.MustChangePassword {
		t.Error("must_change_password = false, want true after admin password reset")
	}
}
