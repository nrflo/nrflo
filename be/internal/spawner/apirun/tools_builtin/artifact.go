package tools_builtin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"be/internal/artifact"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/ws"
)

const maxArtifactBytes = 32 * 1024 * 1024

// artifactAddHandler implements artifact_add.
type artifactAddHandler struct{}

func (artifactAddHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "artifact_add",
		Description: "Upload an artifact for the current workflow instance. Provide either content (UTF-8 text) or content_b64 (base64-encoded binary), not both.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"name":{"type":"string","description":"Artifact name"},
"content":{"type":"string","description":"UTF-8 text content (mutually exclusive with content_b64)"},
"content_b64":{"type":"string","description":"Base64-encoded binary content (mutually exclusive with content)"},
"content_type":{"type":"string","description":"MIME type (optional)"}
},
"required":["name"],
"additionalProperties":false
}`),
	}
}

func (artifactAddHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Name        string `json:"name"`
		Content     string `json:"content"`
		ContentB64  string `json:"content_b64"`
		ContentType string `json:"content_type"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if args.Name == "" {
		return "name is required", true, nil
	}
	if args.Content == "" && args.ContentB64 == "" {
		return "either content or content_b64 is required", true, nil
	}
	if args.Content != "" && args.ContentB64 != "" {
		return "content and content_b64 are mutually exclusive", true, nil
	}
	if env.ArtifactSvc == nil {
		return missingService("artifact")
	}

	var data []byte
	if args.ContentB64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(args.ContentB64)
		if err != nil {
			return "invalid base64 content_b64: " + err.Error(), true, nil
		}
		data = decoded
	} else {
		data = []byte(args.Content)
	}

	if len(data) > maxArtifactBytes {
		return "artifact too large: max 32 MiB", true, nil
	}

	a, err := env.ArtifactSvc.AddFromAgent(ctx, env.SessionID, env.ProjectID, env.WorkflowInstanceID, args.Name, args.ContentType, data)
	if err != nil {
		return err.Error(), true, nil
	}

	service.BroadcastFromCtx(env.WSHub, ws.EventArtifactCreated, service.BroadcastCtx{ProjectID: env.ProjectID}, map[string]interface{}{
		"artifact_id":          a.ID,
		"workflow_instance_id": env.WorkflowInstanceID,
		"name":                 a.Name,
	})

	out, _ := json.Marshal(map[string]string{"id": a.ID, "name": a.Name})
	return string(out), false, nil
}

// artifactListHandler implements artifact_list.
type artifactListHandler struct{}

func (artifactListHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "artifact_list",
		Description: "List artifacts for the current workflow instance.",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}
}

func (artifactListHandler) Invoke(ctx context.Context, env apirun.ToolEnv, _ json.RawMessage) (string, bool, error) {
	if env.ArtifactSvc == nil {
		return missingService("artifact")
	}

	artifacts, err := env.ArtifactSvc.List(ctx, env.WorkflowInstanceID)
	if err != nil {
		return err.Error(), true, nil
	}

	type item struct {
		Name        string `json:"name"`
		SizeBytes   int64  `json:"size_bytes"`
		ContentType string `json:"content_type,omitempty"`
		Source      string `json:"source"`
	}
	result := make([]item, 0, len(artifacts))
	for _, a := range artifacts {
		result = append(result, item{
			Name:        a.Name,
			SizeBytes:   a.SizeBytes,
			ContentType: a.ContentType,
			Source:      a.Source,
		})
	}
	out, err := json.Marshal(result)
	if err != nil {
		return err.Error(), true, nil
	}
	return string(out), false, nil
}

// artifactGetHandler implements artifact_get.
type artifactGetHandler struct{}

func (artifactGetHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "artifact_get",
		Description: "Materialize an artifact to the stage dir and return its absolute path.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"name":{"type":"string","description":"Artifact name"}
},
"required":["name"],
"additionalProperties":false
}`),
	}
}

func (artifactGetHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if args.Name == "" {
		return "name is required", true, nil
	}
	if env.ArtifactSvc == nil {
		return missingService("artifact")
	}

	stageDir, err := ensureStageDir(env.ProjectID, env.WorkflowInstanceID)
	if err != nil {
		return err.Error(), true, nil
	}

	storage, err := env.ArtifactSvc.GetStorage(ctx, env.ProjectID)
	if err != nil {
		return err.Error(), true, nil
	}

	artifactRepo := repo.NewArtifactRepo(env.Pool, env.Clock)
	artifacts, err := artifactRepo.List(env.WorkflowInstanceID)
	if err != nil {
		return err.Error(), true, nil
	}

	for _, a := range artifacts {
		if a.Name == args.Name {
			absPath, matErr := materialize(ctx, a, stageDir, storage)
			if matErr != nil {
				return matErr.Error(), true, nil
			}
			out, _ := json.Marshal(map[string]string{"path": absPath})
			return string(out), false, nil
		}
	}

	return "artifact not found: " + args.Name, true, nil
}

func ensureStageDir(projectID, wfiID string) (string, error) {
	dir := filepath.Join(db.DefaultDataDir(), "projects", projectID, "artifacts", wfiID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func materialize(ctx context.Context, a *model.Artifact, stageDir string, storage artifact.Storage) (string, error) {
	dest := filepath.Join(stageDir, a.Name)
	if fi, err := os.Stat(dest); err == nil && fi.Size() == a.SizeBytes {
		return dest, nil
	}
	rc, err := storage.Get(ctx, a.PathKey)
	if err != nil {
		return "", err
	}
	defer rc.Close()

	tmp, err := os.CreateTemp(stageDir, ".tmp-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, rc); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	tmp.Close()
	if err := os.Rename(tmpName, dest); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	return dest, nil
}
