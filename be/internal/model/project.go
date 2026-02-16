package model

import (
	"database/sql"
	"encoding/json"
	"time"
)

// Project represents a project in the system
type Project struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	RootPath        sql.NullString `json:"-"`
	DefaultWorkflow sql.NullString `json:"-"`
	DefaultBranch   sql.NullString `json:"-"`
	UseGitWorktrees bool           `json:"use_git_worktrees"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// MarshalJSON implements custom JSON marshaling for Project
func (p Project) MarshalJSON() ([]byte, error) {
	var rootPath *string
	if p.RootPath.Valid {
		rootPath = &p.RootPath.String
	}

	var defaultWorkflow *string
	if p.DefaultWorkflow.Valid {
		defaultWorkflow = &p.DefaultWorkflow.String
	}

	var defaultBranch *string
	if p.DefaultBranch.Valid {
		defaultBranch = &p.DefaultBranch.String
	}

	return json.Marshal(&struct {
		ID              string    `json:"id"`
		Name            string    `json:"name"`
		RootPath        *string   `json:"root_path"`
		DefaultWorkflow *string   `json:"default_workflow"`
		DefaultBranch   *string   `json:"default_branch"`
		UseGitWorktrees bool      `json:"use_git_worktrees"`
		CreatedAt       time.Time `json:"created_at"`
		UpdatedAt       time.Time `json:"updated_at"`
	}{
		ID:              p.ID,
		Name:            p.Name,
		RootPath:        rootPath,
		DefaultWorkflow: defaultWorkflow,
		DefaultBranch:   defaultBranch,
		UseGitWorktrees: p.UseGitWorktrees,
		CreatedAt:       p.CreatedAt,
		UpdatedAt:       p.UpdatedAt,
	})
}
