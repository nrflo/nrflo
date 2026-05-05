package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"be/internal/model"
)

// sysAdminID is the ID of the built-in seed admin (flagged system=1 by migration 000086).
const sysAdminID = "usr_admin_seed"

// TestDeleteUser_SystemUser verifies that deleting the seed admin (system=1) returns
// HTTP 400 with {"error":"system_user"}, even when acting as a different admin.
func TestDeleteUser_SystemUser(t *testing.T) {
	as := newAuthServer(t)

	// Create and log in as a second non-system admin to act as the requester.
	seedUser(t, as.pool, "admin2sys@test.com", "pass12345", model.UserRoleAdmin, false)
	cl := newJarClient()
	loginAs(t, cl, as.baseURL, "admin2sys@test.com", "pass12345")

	// Attempt to delete the seed admin (system=1).
	resp := deleteReq(t, cl, as.baseURL+"/api/v1/users/"+sysAdminID)
	defer drain(resp)

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("delete system user: status = %d, want 400", resp.StatusCode)
	}
	var errOut map[string]string
	json.NewDecoder(resp.Body).Decode(&errOut)
	if errOut["error"] != "system_user" {
		t.Errorf("error = %q, want system_user", errOut["error"])
	}
}
