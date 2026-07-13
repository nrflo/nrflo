package tools_builtin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"be/internal/repo"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// ReadDocumentHybridHandler is the cli_interactive variant of read_document
// for CLIs without native document reading (codex). It materializes the
// artifact and returns its absolute path; PNG/JPEG bytes are additionally
// returned as an image media block so the MCP bridge can attach them inline
// as vision input, and PDFs are rasterized to per-page PNG media via the
// server host's pdftoppm (read_document_rasterize.go), falling back to a
// path-only result when the binary is absent or rendering fails. NOT
// registered in Builtins(); swapped in by attachNrfloToolRegistry for
// adapters where SupportsNativeDocRead()==false.
type ReadDocumentHybridHandler struct{}

func (ReadDocumentHybridHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name: "read_document",
		Description: "Load an input artifact (uploaded PDF or image). Images and rendered PDF pages " +
			"are attached to the conversation directly; when a PDF cannot be rendered its " +
			"absolute file path is returned so you can extract the contents from disk. " +
			"Pass the artifact name from #{ARTIFACTS} or artifact_list.",
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

// Invoke is the text-only fallback (required by ToolHandler); the spawner's
// DispatchTool always prefers InvokeMedia.
func (h ReadDocumentHybridHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	out, _, isErr, err := h.InvokeMedia(ctx, env, input)
	return out, isErr, err
}

func (ReadDocumentHybridHandler) InvokeMedia(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, []provider.MediaBlock, bool, error) {
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
		absPath, matErr := materialize(ctx, a, stageDir, storage)
		if matErr != nil {
			return matErr.Error(), nil, true, nil
		}
		kind, mediaType := classifyMedia(a.ContentType, a.Name)
		if kind == "image" && a.SizeBytes <= maxReadDocumentBytes {
			data, readErr := os.ReadFile(absPath)
			if readErr != nil {
				return readErr.Error(), nil, true, nil
			}
			return "Loaded " + a.Name + " (" + mediaType + "); image attached. File also at " + absPath + ".",
				[]provider.MediaBlock{{
					Kind:      kind,
					MediaType: mediaType,
					DataB64:   base64.StdEncoding.EncodeToString(data),
					Name:      a.Name,
				}}, false, nil
		}
		if kind == "document" {
			if bin, lerr := lookPdftoppm(); lerr == nil {
				blocks, truncated, rerr := rasterizePDF(ctx, bin, absPath, a.Name, maxReadDocumentBytes)
				if rerr == nil {
					msg := fmt.Sprintf("Loaded %s (application/pdf); %d page(s) rendered and attached as images.", a.Name, len(blocks))
					if truncated {
						msg += fmt.Sprintf(" Output truncated (%d-page / 32 MiB cap) — the full file is at %s.", rasterMaxPages, absPath)
					} else {
						msg += " File also at " + absPath + "."
					}
					return msg, blocks, false, nil
				}
			}
		}
		out, _ := json.Marshal(map[string]string{
			"path": absPath,
			"note": "Inline viewing is not available for this file in this CLI; extract its contents from the path (e.g. pdftotext for PDFs, or rasterize pages to PNG).",
		})
		return string(out), nil, false, nil
	}

	return "artifact not found: " + args.Name, nil, true, nil
}
