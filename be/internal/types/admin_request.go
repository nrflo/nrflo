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

// CLIModelCreateRequest is the request for creating a CLI model
type CLIModelCreateRequest struct {
	ID               string   `json:"id"`
	CLIType          string   `json:"cli_type"`
	DisplayName      string   `json:"display_name"`
	MappedModel      string   `json:"mapped_model"`
	ReasoningEffort  string   `json:"reasoning_effort"`
	SupportedEfforts []string `json:"supported_efforts"`
	FallbackModels   string   `json:"fallback_models"`
	ContextLength    int      `json:"context_length"`
}

// CLIModelUpdateRequest is the request for updating a CLI model
type CLIModelUpdateRequest struct {
	DisplayName      *string   `json:"display_name,omitempty"`
	MappedModel      *string   `json:"mapped_model,omitempty"`
	ReasoningEffort  *string   `json:"reasoning_effort,omitempty"`
	SupportedEfforts *[]string `json:"supported_efforts,omitempty"`
	FallbackModels   *string   `json:"fallback_models,omitempty"`
	ContextLength    *int      `json:"context_length,omitempty"`
	Enabled          *bool     `json:"enabled,omitempty"`
}

// APIModelCreateRequest is the request for creating an API model
type APIModelCreateRequest struct {
	ID               string   `json:"id"`
	Provider         string   `json:"provider"`
	DisplayName      string   `json:"display_name"`
	MappedModel      string   `json:"mapped_model"`
	ReasoningEffort  string   `json:"reasoning_effort"`
	SupportedEfforts []string `json:"supported_efforts"`
	ContextLength    int      `json:"context_length"`
}

// APIModelUpdateRequest is the request for updating an API model
type APIModelUpdateRequest struct {
	DisplayName      *string   `json:"display_name,omitempty"`
	MappedModel      *string   `json:"mapped_model,omitempty"`
	ReasoningEffort  *string   `json:"reasoning_effort,omitempty"`
	SupportedEfforts *[]string `json:"supported_efforts,omitempty"`
	ContextLength    *int      `json:"context_length,omitempty"`
	Enabled          *bool     `json:"enabled,omitempty"`
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
