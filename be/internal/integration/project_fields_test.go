package integration

import (
	"testing"

	"be/internal/db"
)

// TestProjectFields_CreateGetList covers create + GET + list for the
// use_git_worktrees (bool) and default_branch (nullable string) fields,
// including the default/null cases and the raw DB-column values.
func TestProjectFields_CreateGetList(t *testing.T) {
	cases := []struct {
		name string
		id   string
		body string
		// wantField is the JSON field to assert in GET/list responses.
		wantField string
		wantValue interface{}
		// dbCol/wantDB asserts the raw stored value (int for bool, *string for branch).
		dbCol  string
		wantDB interface{}
	}{
		{name: "worktrees true", id: "pf-wt-true", body: `{"id":"pf-wt-true","name":"WT True","use_git_worktrees":true}`, wantField: "use_git_worktrees", wantValue: true, dbCol: "use_git_worktrees", wantDB: 1},
		{name: "worktrees default false", id: "pf-wt-default", body: `{"id":"pf-wt-default","name":"WT Default"}`, wantField: "use_git_worktrees", wantValue: false, dbCol: "use_git_worktrees", wantDB: 0},
		{name: "worktrees explicit false", id: "pf-wt-false", body: `{"id":"pf-wt-false","name":"WT False","use_git_worktrees":false}`, wantField: "use_git_worktrees", wantValue: false, dbCol: "use_git_worktrees", wantDB: 0},
		{name: "branch set", id: "pf-br-set", body: `{"id":"pf-br-set","name":"Branch Set","default_branch":"develop"}`, wantField: "default_branch", wantValue: "develop", dbCol: "default_branch", wantDB: "develop"},
		{name: "branch null", id: "pf-br-null", body: `{"id":"pf-br-null","name":"Branch Null"}`, wantField: "default_branch", wantValue: nil, dbCol: "default_branch", wantDB: nil},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			baseURL, client, dbPath := projectFieldsServerWithDB(t)
			createProjectJSON(t, client, baseURL, tc.body)

			// GET response carries the field.
			got := getProjectMap(t, client, baseURL, tc.id)
			v, ok := got[tc.wantField]
			if !ok {
				t.Fatalf("expected %s field in GET response", tc.wantField)
			}
			if v != tc.wantValue {
				t.Fatalf("GET %s = %v, want %v", tc.wantField, v, tc.wantValue)
			}

			// List response carries the field.
			projects := listProjectMaps(t, client, baseURL)
			var found bool
			for _, p := range projects {
				if p["id"] == tc.id {
					found = true
					if p[tc.wantField] != tc.wantValue {
						t.Fatalf("list %s = %v, want %v", tc.wantField, p[tc.wantField], tc.wantValue)
					}
				}
			}
			if !found {
				t.Fatalf("%s not found in list response", tc.id)
			}

			// Raw DB column value.
			database, err := db.Open(dbPath)
			if err != nil {
				t.Fatalf("reopen DB: %v", err)
			}
			defer database.Close()

			switch want := tc.wantDB.(type) {
			case int:
				var got int
				if err := database.QueryRow("SELECT "+tc.dbCol+" FROM projects WHERE id = ?", tc.id).Scan(&got); err != nil {
					t.Fatalf("query %s: %v", tc.dbCol, err)
				}
				if got != want {
					t.Fatalf("DB %s = %d, want %d", tc.dbCol, got, want)
				}
			case string:
				var got *string
				if err := database.QueryRow("SELECT "+tc.dbCol+" FROM projects WHERE id = ?", tc.id).Scan(&got); err != nil {
					t.Fatalf("query %s: %v", tc.dbCol, err)
				}
				if got == nil || *got != want {
					t.Fatalf("DB %s = %v, want %q", tc.dbCol, got, want)
				}
			case nil:
				var got *string
				if err := database.QueryRow("SELECT "+tc.dbCol+" FROM projects WHERE id = ?", tc.id).Scan(&got); err != nil {
					t.Fatalf("query %s: %v", tc.dbCol, err)
				}
				if got != nil {
					t.Fatalf("DB %s = %q, want nil", tc.dbCol, *got)
				}
			}
		})
	}
}

// TestProjectFields_Update covers PATCH-driven mutations: setting/toggling the
// boolean worktrees flag, setting/clearing default_branch, partial updates that
// must not reset untouched fields, and multi-field updates.
func TestProjectFields_Update(t *testing.T) {
	t.Run("toggle worktrees both directions", func(t *testing.T) {
		baseURL, client, dbPath := projectFieldsServerWithDB(t)
		createProjectJSON(t, client, baseURL, `{"id":"upd-wt","name":"Toggle"}`)

		res := patchProjectMap(t, client, baseURL, "upd-wt", `{"use_git_worktrees":true}`)
		if res["use_git_worktrees"] != true {
			t.Fatalf("after set true: use_git_worktrees = %v", res["use_git_worktrees"])
		}
		assertWorktreesDB(t, dbPath, "upd-wt", 1)

		res = patchProjectMap(t, client, baseURL, "upd-wt", `{"use_git_worktrees":false}`)
		if res["use_git_worktrees"] != false {
			t.Fatalf("after set false: use_git_worktrees = %v", res["use_git_worktrees"])
		}
		assertWorktreesDB(t, dbPath, "upd-wt", 0)
	})

	t.Run("partial update does not reset worktrees", func(t *testing.T) {
		baseURL, client, dbPath := projectFieldsServerWithDB(t)
		createProjectJSON(t, client, baseURL, `{"id":"upd-noreset","name":"No Reset","use_git_worktrees":true}`)

		res := patchProjectMap(t, client, baseURL, "upd-noreset", `{"name":"Updated Name"}`)
		if res["use_git_worktrees"] != true {
			t.Fatalf("use_git_worktrees should remain true, got %v", res["use_git_worktrees"])
		}
		assertWorktreesDB(t, dbPath, "upd-noreset", 1)
	})

	t.Run("set default_branch", func(t *testing.T) {
		baseURL, client := projectFieldsServer(t)
		createProjectJSON(t, client, baseURL, `{"id":"upd-br-set","name":"Branch"}`)

		res := patchProjectMap(t, client, baseURL, "upd-br-set", `{"default_branch":"feature-branch"}`)
		if res["default_branch"] != "feature-branch" {
			t.Fatalf("default_branch = %v, want feature-branch", res["default_branch"])
		}
	})

	t.Run("clear default_branch", func(t *testing.T) {
		baseURL, client, dbPath := projectFieldsServerWithDB(t)
		createProjectJSON(t, client, baseURL, `{"id":"upd-br-clear","name":"Clear","default_branch":"main"}`)

		patchProjectMap(t, client, baseURL, "upd-br-clear", `{"default_branch":""}`)

		database, err := db.Open(dbPath)
		if err != nil {
			t.Fatalf("reopen DB: %v", err)
		}
		defer database.Close()
		var branch *string
		if err := database.QueryRow("SELECT default_branch FROM projects WHERE id = ?", "upd-br-clear").Scan(&branch); err != nil {
			t.Fatalf("query default_branch: %v", err)
		}
		// Empty string in the update is stored as an empty string, not NULL.
		if branch == nil || *branch != "" {
			t.Fatalf("default_branch = %v, want empty string", branch)
		}
	})

	t.Run("multi-field update", func(t *testing.T) {
		baseURL, client := projectFieldsServer(t)
		createProjectJSON(t, client, baseURL, `{"id":"upd-multi","name":"Multi","default_branch":"main"}`)

		res := patchProjectMap(t, client, baseURL, "upd-multi", `{"name":"Updated Multi","default_branch":"develop","use_git_worktrees":true}`)
		if res["name"] != "Updated Multi" {
			t.Errorf("name = %v, want Updated Multi", res["name"])
		}
		if res["default_branch"] != "develop" {
			t.Errorf("default_branch = %v, want develop", res["default_branch"])
		}
		if res["use_git_worktrees"] != true {
			t.Errorf("use_git_worktrees = %v, want true", res["use_git_worktrees"])
		}
	})
}

func assertWorktreesDB(t *testing.T, dbPath, id string, want int) {
	t.Helper()
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen DB: %v", err)
	}
	defer database.Close()
	var got int
	if err := database.QueryRow("SELECT use_git_worktrees FROM projects WHERE id = ?", id).Scan(&got); err != nil {
		t.Fatalf("query use_git_worktrees: %v", err)
	}
	if got != want {
		t.Fatalf("DB use_git_worktrees = %d, want %d", got, want)
	}
}

// TestProjectFields_DefaultWorkflowIgnored verifies that the removed
// default_workflow field is silently ignored on create/update and never
// appears in GET or list responses.
func TestProjectFields_DefaultWorkflowIgnored(t *testing.T) {
	baseURL, client := projectFieldsServer(t)

	// Create with default_workflow present — accepted (201), field ignored.
	createProjectJSON(t, client, baseURL, `{"id":"pf-dw","name":"DW","default_workflow":"feature"}`)

	got := getProjectMap(t, client, baseURL, "pf-dw")
	if _, ok := got["default_workflow"]; ok {
		t.Errorf("GET response must not contain default_workflow, got %v", got["default_workflow"])
	}

	// PATCH with default_workflow present — accepted (200), field ignored, name still updates.
	res := patchProjectMap(t, client, baseURL, "pf-dw", `{"name":"DW Updated","default_workflow":"bugfix"}`)
	if res["name"] != "DW Updated" {
		t.Errorf("name = %v, want DW Updated", res["name"])
	}
	if _, ok := res["default_workflow"]; ok {
		t.Errorf("PATCH response must not contain default_workflow, got %v", res["default_workflow"])
	}

	// List response must not carry default_workflow on any project.
	for _, p := range listProjectMaps(t, client, baseURL) {
		if _, ok := p["default_workflow"]; ok {
			t.Errorf("project %v must not contain default_workflow", p["id"])
		}
	}
}
