package tools_builtin

import (
	"context"
	"encoding/json"

	"be/internal/repo"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// ReadDocumentPathHandler is the api-via-cli variant of read_document.
// Instead of inlining the document bytes into a content block, it
// materializes the artifact to the stage dir and returns its absolute path
// so the Claude CLI (which has native Read) can read it directly.
// NOT registered in Builtins(); swapped in only by the hybrid prep.
type ReadDocumentPathHandler struct{}

func (ReadDocumentPathHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "read_document",
		Description: "Materialize the document artifact and return its path; use Read on the path to view it.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"name":{"type":"string","description":"Artifact name to materialize"}
},
"required":["name"],
"additionalProperties":false
}`),
	}
}

func (ReadDocumentPathHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
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
