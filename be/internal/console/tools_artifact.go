package console

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"

	"be/internal/model"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// maxInlineArtifactBytes caps artifact_get's inline body: the console caller
// is an HTTP client, not a co-located process, so there is no stage dir to
// materialize into (unlike the registry artifact_get's materialize()).
const maxInlineArtifactBytes = 1 << 20 // 1 MiB

// artifactListHandler implements the console artifact_list: it takes an
// EXPLICIT instance_id (the registry builtin reads env.WorkflowInstanceID,
// which is empty for a console session).
type artifactListHandler struct{ d Deps }

func (artifactListHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "artifact_list",
		Description: "List artifacts for a workflow instance.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{"instance_id":{"type":"string"}},
"required":["instance_id"],
"additionalProperties":false
}`),
	}
}

func (h artifactListHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		InstanceID string `json:"instance_id"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if args.InstanceID == "" {
		return "instance_id is required", true, nil
	}
	if h.d.ArtifactSvc == nil {
		return missingService("artifact")
	}
	if _, err := loadGuardedInstance(h.d, env.ProjectID, args.InstanceID); err != nil {
		return err.Error(), true, nil
	}
	artifacts, err := h.d.ArtifactSvc.List(ctx, args.InstanceID)
	if err != nil {
		return err.Error(), true, nil
	}
	type item struct {
		Name        string `json:"name"`
		SizeBytes   int64  `json:"size_bytes"`
		ContentType string `json:"content_type,omitempty"`
		Source      string `json:"source"`
	}
	out := make([]item, 0, len(artifacts))
	for _, a := range artifacts {
		out = append(out, item{Name: a.Name, SizeBytes: a.SizeBytes, ContentType: a.ContentType, Source: a.Source})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return err.Error(), true, nil
	}
	return string(b), false, nil
}

// artifactGetHandler implements the console artifact_get: returns content
// inline (text for text/* + utf8-valid bodies, base64 otherwise, capped at
// maxInlineArtifactBytes with a truncated flag).
type artifactGetHandler struct{ d Deps }

func (artifactGetHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "artifact_get",
		Description: "Fetch an artifact's content inline (text when UTF-8/text, base64 otherwise; truncated at 1 MiB).",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{"instance_id":{"type":"string"},"name":{"type":"string"}},
"required":["instance_id","name"],
"additionalProperties":false
}`),
	}
}

func (h artifactGetHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		InstanceID string `json:"instance_id"`
		Name       string `json:"name"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if args.InstanceID == "" || args.Name == "" {
		return "instance_id and name are required", true, nil
	}
	if h.d.ArtifactSvc == nil {
		return missingService("artifact")
	}
	if _, err := loadGuardedInstance(h.d, env.ProjectID, args.InstanceID); err != nil {
		return err.Error(), true, nil
	}
	artifacts, err := h.d.ArtifactSvc.List(ctx, args.InstanceID)
	if err != nil {
		return err.Error(), true, nil
	}
	var found *model.Artifact
	for _, a := range artifacts {
		if a.Name == args.Name {
			found = a
			break
		}
	}
	if found == nil {
		return "artifact not found: " + args.Name, true, nil
	}
	rc, err := h.d.ArtifactSvc.Open(ctx, found)
	if err != nil {
		return err.Error(), true, nil
	}
	defer rc.Close()

	data, err := io.ReadAll(io.LimitReader(rc, maxInlineArtifactBytes+1))
	if err != nil {
		return err.Error(), true, nil
	}
	truncated := len(data) > maxInlineArtifactBytes
	if truncated {
		data = data[:maxInlineArtifactBytes]
	}

	resp := map[string]interface{}{
		"name":         found.Name,
		"content_type": found.ContentType,
		"size_bytes":   found.SizeBytes,
		"truncated":    truncated,
	}
	if isInlineText(found.ContentType, data) {
		resp["encoding"] = "text"
		resp["content"] = string(data)
	} else {
		resp["encoding"] = "base64"
		resp["content_b64"] = base64.StdEncoding.EncodeToString(data)
	}
	b, err := json.Marshal(resp)
	if err != nil {
		return err.Error(), true, nil
	}
	return string(b), false, nil
}

func isInlineText(contentType string, data []byte) bool {
	if strings.HasPrefix(contentType, "text/") {
		return true
	}
	return utf8.Valid(data)
}
