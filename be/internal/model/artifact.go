package model

import "time"

const (
	ArtifactSourceInput    = "input"
	ArtifactSourceAgent    = "agent"
	ArtifactTypeInternal   = "internal"
	ArtifactTypeS3         = "s3"
	ArtifactTypeCloudflareR2 = "cloudflare_r2"
)

// Artifact is a file stored by a workflow agent or supplied as input.
type Artifact struct {
	ID                 string    `json:"id"`
	ProjectID          string    `json:"project_id"`
	WorkflowInstanceID string    `json:"workflow_instance_id"`
	Name               string    `json:"name"`
	Type               string    `json:"type"`
	PathKey            string    `json:"path_key"`
	SizeBytes          int64     `json:"size_bytes"`
	ContentType        string    `json:"content_type,omitempty"`
	Source             string    `json:"source"`
	CreatedBySession   string    `json:"created_by_session,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}
