package tools_builtin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"

	"be/internal/repo"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// maxReadDocumentBytes caps how large a file read_document will inline into a
// tool_result. Anthropic limits documents to ~32 MB; dd-buyer caps uploads at
// 25 MiB, so 32 MiB is a safe ceiling.
const maxReadDocumentBytes = 32 * 1024 * 1024

// readDocumentHandler implements read_document: it materializes a named input
// artifact and returns its bytes as an image/document content block so the
// model can read it natively (OCR scanned PDFs, photos, etc.). PDFs become a
// document block; PNG/JPEG become image blocks. Other types fall back to a
// text error.
type readDocumentHandler struct{}

func (readDocumentHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name: "read_document",
		Description: "Load an input artifact (uploaded PDF or image) so you can read its contents " +
			"directly — including scanned pages and photos. Pass the artifact name from " +
			"#{ARTIFACTS} or artifact_list. PDFs and PNG/JPEG images are supported.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"name":{"type":"string","description":"Artifact name to load"}
},
"required":["name"],
"additionalProperties":false
}`),
	}
}

// Invoke is the text-only fallback (required by ToolHandler). It defers to the
// media path and discards the media, returning a hint. The runner always calls
// InvokeMedia for this handler, so this is only hit if the media interface is
// bypassed.
func (h readDocumentHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	out, _, isErr, err := h.InvokeMedia(ctx, env, input)
	return out, isErr, err
}

func (readDocumentHandler) InvokeMedia(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, []provider.MediaBlock, bool, error) {
	var args struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "invalid arguments: " + err.Error(), nil, true, nil
	}
	if args.Name == "" {
		return "name is required", nil, true, nil
	}
	if env.ArtifactSvc == nil {
		return "artifact service unavailable", nil, true, nil
	}

	stageDir, err := ensureStageDir(env.ProjectID, env.WorkflowInstanceID)
	if err != nil {
		return err.Error(), nil, true, nil
	}
	storage, err := env.ArtifactSvc.GetStorage(ctx, env.ProjectID)
	if err != nil {
		return err.Error(), nil, true, nil
	}
	artifactRepo := repo.NewArtifactRepo(env.Pool, env.Clock)
	artifacts, err := artifactRepo.List(env.WorkflowInstanceID)
	if err != nil {
		return err.Error(), nil, true, nil
	}

	for _, a := range artifacts {
		if a.Name != args.Name {
			continue
		}
		if a.SizeBytes > maxReadDocumentBytes {
			return "artifact too large to read inline (max 32 MiB)", nil, true, nil
		}
		absPath, matErr := materialize(ctx, a, stageDir, storage)
		if matErr != nil {
			return matErr.Error(), nil, true, nil
		}
		data, readErr := os.ReadFile(absPath)
		if readErr != nil {
			return readErr.Error(), nil, true, nil
		}
		kind, mediaType := classifyMedia(a.ContentType, a.Name)
		if kind == "" {
			return "unsupported document type for " + a.Name + " (only PDF, PNG, JPEG can be read)", nil, true, nil
		}
		return "Loaded " + a.Name + " (" + mediaType + ").",
			[]provider.MediaBlock{{
				Kind:      kind,
				MediaType: mediaType,
				DataB64:   base64.StdEncoding.EncodeToString(data),
				Name:      a.Name,
			}}, false, nil
	}

	return "artifact not found: " + args.Name, nil, true, nil
}

// classifyMedia maps a stored artifact's content type / filename to a media
// block kind. Returns ("","") for unsupported types.
func classifyMedia(contentType, name string) (kind, mediaType string) {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	switch ct {
	case "application/pdf":
		return "document", "application/pdf"
	case "image/png":
		return "image", "image/png"
	case "image/jpeg", "image/jpg":
		return "image", "image/jpeg"
	}
	switch {
	case strings.HasSuffix(strings.ToLower(name), ".pdf"):
		return "document", "application/pdf"
	case strings.HasSuffix(strings.ToLower(name), ".png"):
		return "image", "image/png"
	case strings.HasSuffix(strings.ToLower(name), ".jpg"), strings.HasSuffix(strings.ToLower(name), ".jpeg"):
		return "image", "image/jpeg"
	}
	return "", ""
}
