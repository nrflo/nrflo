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
	UseGitWorktrees  bool   `json:"use_git_worktrees"`
	ClaudeSafetyHook string `json:"-"` // Loaded from config table, not projects table
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// MarshalJSON implements custom JSON marshaling for Project
func (p Project) MarshalJSON() ([]byte, error) {
	var rootPath *string
	if p.RootPath.Valid {
		rootPath = &p.RootPath.String
	}

	var defaultBranch *string
	if p.DefaultBranch.Valid {
		defaultBranch = &p.DefaultBranch.String
	}

	var claudeSafetyHook *string
	if p.ClaudeSafetyHook != "" {
		claudeSafetyHook = &p.ClaudeSafetyHook
	}

	return json.Marshal(&struct {
		ID              string    `json:"id"`
		Name            string    `json:"name"`
		RootPath        *string   `json:"root_path"`
		DefaultBranch   *string   `json:"default_branch"`
		UseGitWorktrees  bool      `json:"use_git_worktrees"`
		ClaudeSafetyHook *string   `json:"claude_safety_hook"`
		CreatedAt       time.Time `json:"created_at"`
		UpdatedAt       time.Time `json:"updated_at"`
	}{
		ID:              p.ID,
		Name:            p.Name,
		RootPath:        rootPath,
		DefaultBranch:   defaultBranch,
		UseGitWorktrees:  p.UseGitWorktrees,
		ClaudeSafetyHook: claudeSafetyHook,
		CreatedAt:       p.CreatedAt,
		UpdatedAt:       p.UpdatedAt,
	})
}
