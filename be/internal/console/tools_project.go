package console

import (
	"context"
	"encoding/json"
	"strings"

	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// projectListHandler implements project_list.
type projectListHandler struct{ d Deps }

func (projectListHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "project_list",
		Description: "List all projects (id, name, root_path).",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}
}

func (h projectListHandler) Invoke(ctx context.Context, env apirun.ToolEnv, _ json.RawMessage) (string, bool, error) {
	if h.d.Pool == nil {
		return missingService("pool")
	}
	projects, err := repo.NewProjectRepo(h.d.Pool, h.d.Clock).List()
	if err != nil {
		return err.Error(), true, nil
	}
	type item struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		RootPath string `json:"root_path,omitempty"`
	}
	out := make([]item, 0, len(projects))
	for _, p := range projects {
		out = append(out, item{ID: p.ID, Name: p.Name, RootPath: p.RootPath.String})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return err.Error(), true, nil
	}
	return string(b), false, nil
}

// projectStatusHandler implements project_status, reusing the same
// StatusService the REST GET /api/v1/status handler calls.
type projectStatusHandler struct{ d Deps }

func (projectStatusHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "project_status",
		Description: "Dashboard summary for a project: pending tickets, blocked/ready counts, recently closed tickets. `project` is only honored for a global-scope console session.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{"project":{"type":"string"}},
"additionalProperties":false
}`),
	}
}

func (h projectStatusHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Project string `json:"project"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return invalidArgs(err)
		}
	}
	if h.d.Pool == nil {
		return missingService("pool")
	}
	projectID := env.ProjectID
	if args.Project != "" && strings.EqualFold(env.ProjectID, service.GlobalProjectID) {
		projectID = args.Project
	}
	status, err := service.NewStatusService(h.d.Pool, h.d.Clock).ProjectStatus(projectID, 10)
	if err != nil {
		return err.Error(), true, nil
	}
	out, err := json.Marshal(status)
	if err != nil {
		return err.Error(), true, nil
	}
	return string(out), false, nil
}
