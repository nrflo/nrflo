package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/types"
)

// seedTieringHandlerDef inserts one agent_definitions row for tiering handler
// tests, creating its project+workflow if absent.
func seedTieringHandlerDef(t *testing.T, as *authServer, projectID, workflowID, defID, model string, consultant bool) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := as.pool.Exec(`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, '/tmp', ?, ?)`,
		projectID, projectID, now, now); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := as.pool.Exec(`INSERT OR IGNORE INTO workflows (project_id, id, description, scope_type, groups, created_at, updated_at)
		VALUES (?, ?, '', 'ticket', '[]', ?, ?)`, projectID, workflowID, now, now); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	consultantInt := 0
	if consultant {
		consultantInt = 1
	}
	if _, err := as.pool.Exec(`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, layer, consultant, created_at, updated_at)
		VALUES (?, ?, ?, ?, 20, '', 0, ?, ?, ?)`, defID, projectID, workflowID, model, consultantInt, now, now); err != nil {
		t.Fatalf("seed agent def: %v", err)
	}
}

// bearerGet issues a GET with a raw Authorization: Bearer header (no cookie jar).
func bearerGet(t *testing.T, url, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

func TestHandleTieringReport_Admin(t *testing.T) {
	as := newAuthServer(t)
	mustLogin(t, as, adminEmail, adminPass)
	seedTieringHandlerDef(t, as, "tr-proj", "feature", "implementor", "opus-4-8", false)

	resp, err := as.client.Get(as.baseURL + "/api/v1/admin/tiering-report")
	if err != nil {
		t.Fatalf("GET tiering-report: %v", err)
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var report types.TieringReport
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if report.Markdown == "" {
		t.Error("Markdown must not be empty")
	}
	found := false
	for _, p := range report.Projects {
		if p.ProjectID != "tr-proj" {
			continue
		}
		for _, d := range p.Defs {
			if d.WorkflowID == "feature" && d.DefID == "implementor" {
				found = true
				if d.RecommendedModel != "sonnet-5" {
					t.Errorf("RecommendedModel = %q, want sonnet-5", d.RecommendedModel)
				}
			}
		}
	}
	if !found {
		t.Error("seeded implementor def not found in report")
	}
}

func TestHandleTieringReport_NonAdminSession403(t *testing.T) {
	as := newAuthServer(t)
	seedUser(t, as.pool, "viewer@tiering.com", "pass12345", model.UserRoleViewer, false)
	cl := newJarClient()
	loginAs(t, cl, as.baseURL, "viewer@tiering.com", "pass12345")

	resp, err := cl.Get(as.baseURL + "/api/v1/admin/tiering-report")
	if err != nil {
		t.Fatalf("GET tiering-report: %v", err)
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("non-admin session: status = %d, want 403", resp.StatusCode)
	}
}

func TestHandleTieringReport_Bearer403(t *testing.T) {
	as := newAuthServer(t)
	seedTokenSession(t, as.srv, "tr-bearer-proj", "tr-tok", model.AgentSessionRunning)

	resp := bearerGet(t, as.baseURL+"/api/v1/admin/tiering-report", "tr-tok")
	defer drain(resp)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("bearer request: status = %d, want 403 (bearer never satisfies requireAdmin)", resp.StatusCode)
	}
}

func TestHandleApplyTiering_AppliesAndFlags(t *testing.T) {
	as := newAuthServer(t)
	mustLogin(t, as, adminEmail, adminPass)
	seedTieringHandlerDef(t, as, "ta-proj", "feature", "implementor", "opus-4-8", false)
	seedTieringHandlerDef(t, as, "ta-proj", "feature", "qa-verifier", "opus-4-8", true)

	body := `{"confirmations":[{"project_id":"ta-proj","confirm_all":true}]}`
	resp := postJSON(t, as.client, as.baseURL+"/api/v1/admin/tiering-apply", body)
	defer drain(resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var result types.TieringApplyResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	var appliedImpl, skippedConsultant bool
	for _, o := range result.Applied {
		if o.ProjectID == "ta-proj" && o.DefID == "implementor" && o.Outcome == "applied" {
			appliedImpl = true
		}
	}
	for _, o := range result.Skipped {
		if o.ProjectID == "ta-proj" && o.DefID == "qa-verifier" && o.Outcome == "skipped-consultant" {
			skippedConsultant = true
		}
	}
	if !appliedImpl {
		t.Errorf("implementor not reported applied: %+v", result.Applied)
	}
	if !skippedConsultant {
		t.Errorf("consultant qa-verifier not reported skipped-consultant: %+v", result.Skipped)
	}

	var dbModel string
	if err := as.pool.QueryRow(`SELECT model FROM agent_definitions WHERE project_id='ta-proj' AND workflow_id='feature' AND id='implementor'`).Scan(&dbModel); err != nil {
		t.Fatalf("query model: %v", err)
	}
	if dbModel != "sonnet-5" {
		t.Errorf("implementor model in DB = %q, want sonnet-5", dbModel)
	}
}

func TestHandleApplyTiering_MissingConfirmations400(t *testing.T) {
	as := newAuthServer(t)
	mustLogin(t, as, adminEmail, adminPass)

	resp := postJSON(t, as.client, as.baseURL+"/api/v1/admin/tiering-apply", `{}`)
	defer drain(resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHandleApplyTiering_NonAdminSession403(t *testing.T) {
	as := newAuthServer(t)
	seedUser(t, as.pool, "viewer2@tiering.com", "pass12345", model.UserRoleViewer, false)
	cl := newJarClient()
	loginAs(t, cl, as.baseURL, "viewer2@tiering.com", "pass12345")

	resp := postJSON(t, cl, as.baseURL+"/api/v1/admin/tiering-apply", `{"confirmations":[{"project_id":"x","confirm_all":true}]}`)
	defer drain(resp)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("non-admin session: status = %d, want 403", resp.StatusCode)
	}
}
