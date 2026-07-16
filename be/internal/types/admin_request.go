package types

// DefaultTemplateCreateRequest is the request for creating a default template
type DefaultTemplateCreateRequest struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Template string `json:"template"`
}

// DefaultTemplateUpdateRequest is the request for updating a default template
type DefaultTemplateUpdateRequest struct {
	Name     *string `json:"name,omitempty"`
	Type     *string `json:"type,omitempty"`
	Template *string `json:"template,omitempty"`
}

// ModelCreateRequest creates one provider model with at least one mode.
type ModelCreateRequest struct {
	ID             string   `json:"id"`
	Provider       string   `json:"provider"`
	DisplayName    string   `json:"display_name"`
	CLIModel       string   `json:"cli_model"`
	APIModel       string   `json:"api_model"`
	CLIEfforts     []string `json:"cli_efforts"`
	APIEfforts     []string `json:"api_efforts"`
	CLIContext     int      `json:"cli_context"`
	APIContext     int      `json:"api_context"`
	FallbackModels string   `json:"fallback_models"`
	DefaultEffort  string   `json:"default_effort"`
}

// ModelUpdateRequest partially updates a provider model.
type ModelUpdateRequest struct {
	DisplayName    *string   `json:"display_name,omitempty"`
	CLIModel       *string   `json:"cli_model,omitempty"`
	APIModel       *string   `json:"api_model,omitempty"`
	CLIEfforts     *[]string `json:"cli_efforts,omitempty"`
	APIEfforts     *[]string `json:"api_efforts,omitempty"`
	CLIContext     *int      `json:"cli_context,omitempty"`
	APIContext     *int      `json:"api_context,omitempty"`
	FallbackModels *string   `json:"fallback_models,omitempty"`
	DefaultEffort  *string   `json:"default_effort,omitempty"`
	Enabled        *bool     `json:"enabled,omitempty"`
}

// InputArtifactRef references a staged upload to attach to a workflow run.
type InputArtifactRef struct {
	UploadID string `json:"upload_id"`
	Name     string `json:"name,omitempty"`
}

// ArtifactUploadResponse is returned after staging an upload.
type ArtifactUploadResponse struct {
	UploadID    string `json:"upload_id"`
	Name        string `json:"name"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentType string `json:"content_type"`
}

// ArtifactDTO is the API representation of a stored artifact.
type ArtifactDTO struct {
	ID                 string `json:"id"`
	ProjectID          string `json:"project_id"`
	WorkflowInstanceID string `json:"workflow_instance_id"`
	Name               string `json:"name"`
	Type               string `json:"type"`
	SizeBytes          int64  `json:"size_bytes"`
	ContentType        string `json:"content_type,omitempty"`
	Source             string `json:"source"`
	CreatedBySession   string `json:"created_by_session,omitempty"`
	CreatedAt          string `json:"created_at"`
}
