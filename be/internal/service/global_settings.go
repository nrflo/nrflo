package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"be/internal/artifact"
	"be/internal/clock"
	"be/internal/db"
)

const artifactStorageKey = "artifact_storage"
const workflowCleanupEnabledKey = "workflow_cleanup_enabled"
const sessionRetentionLimitKey = "session_retention_limit"

// GlobalSettingsService provides access to global config key-value store.
type GlobalSettingsService struct {
	pool  *db.Pool
	clock clock.Clock
}

// NewGlobalSettingsService creates a new GlobalSettingsService.
func NewGlobalSettingsService(pool *db.Pool, clk clock.Clock) *GlobalSettingsService {
	return &GlobalSettingsService{pool: pool, clock: clk}
}

// Get returns the value for a config key. Returns "" if not found.
func (s *GlobalSettingsService) Get(key string) (string, error) {
	return s.pool.GetConfig(key)
}

// Set upserts a config key-value pair.
func (s *GlobalSettingsService) Set(key, value string) error {
	return s.pool.SetConfig(key, value)
}

// GetProjectConfig returns the value for a project-scoped config key. Returns "" if not found.
func (s *GlobalSettingsService) GetProjectConfig(projectID, key string) (string, error) {
	return s.pool.GetProjectConfig(projectID, key)
}

// SetProjectConfig upserts a project-scoped config key-value pair.
func (s *GlobalSettingsService) SetProjectConfig(projectID, key, value string) error {
	return s.pool.SetProjectConfig(projectID, key, value)
}

// GetArtifactStorage returns the artifact storage config for a project. Defaults to {mode: internal}.
func (s *GlobalSettingsService) GetArtifactStorage(projectID string) (artifact.Config, error) {
	raw, err := s.pool.GetProjectConfig(projectID, artifactStorageKey)
	if err != nil {
		return artifact.Config{}, err
	}
	if raw == "" {
		return artifact.Config{Mode: artifact.ModeInternal}, nil
	}
	var cfg artifact.Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return artifact.Config{}, err
	}
	return cfg, nil
}

// SetArtifactStorage persists the artifact storage config for a project.
// Returns an error without persisting if mode=s3 (not yet implemented) or R2 fields are missing.
func (s *GlobalSettingsService) SetArtifactStorage(projectID string, cfg artifact.Config) error {
	switch cfg.Mode {
	case artifact.ModeInternal:
		// OK
	case artifact.ModeS3:
		return errors.New("s3 backend not yet implemented")
	case artifact.ModeR2:
		if cfg.AccessKeyRef == artifact.RedactedSentinel || cfg.SecretKeyRef == artifact.RedactedSentinel {
			existing, err := s.GetArtifactStorage(projectID)
			if err != nil {
				return err
			}
			if cfg.AccessKeyRef == artifact.RedactedSentinel {
				cfg.AccessKeyRef = existing.AccessKeyRef
			}
			if cfg.SecretKeyRef == artifact.RedactedSentinel {
				cfg.SecretKeyRef = existing.SecretKeyRef
			}
		}
		var missing []string
		if cfg.AccountID == "" {
			missing = append(missing, "account_id")
		}
		if cfg.Bucket == "" {
			missing = append(missing, "bucket")
		}
		if cfg.AccessKeyRef == "" {
			missing = append(missing, "access_key_ref")
		}
		if cfg.SecretKeyRef == "" {
			missing = append(missing, "secret_key_ref")
		}
		if len(missing) > 0 {
			return fmt.Errorf("cloudflare_r2 requires: %s", strings.Join(missing, ", "))
		}
	default:
		return fmt.Errorf("unknown storage mode %q", cfg.Mode)
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return s.pool.SetProjectConfig(projectID, artifactStorageKey, string(data))
}

// GetArtifactStorageRedacted returns the artifact storage config with secret refs redacted.
func (s *GlobalSettingsService) GetArtifactStorageRedacted(projectID string) (artifact.Config, error) {
	cfg, err := s.GetArtifactStorage(projectID)
	if err != nil {
		return artifact.Config{}, err
	}
	if cfg.AccessKeyRef != "" {
		cfg.AccessKeyRef = artifact.RedactSecretRef(cfg.AccessKeyRef)
	}
	if cfg.SecretKeyRef != "" {
		cfg.SecretKeyRef = artifact.RedactSecretRef(cfg.SecretKeyRef)
	}
	return cfg, nil
}

// GetWorkflowCleanupEnabled returns whether workflow cleanup is enabled for a project.
// Defaults to false if not set.
func (s *GlobalSettingsService) GetWorkflowCleanupEnabled(projectID string) (bool, error) {
	val, err := s.pool.GetProjectConfig(projectID, workflowCleanupEnabledKey)
	if err != nil {
		return false, err
	}
	return val == "true", nil
}

// SetWorkflowCleanupEnabled persists the workflow cleanup enabled flag for a project.
func (s *GlobalSettingsService) SetWorkflowCleanupEnabled(projectID string, enabled bool) error {
	val := "false"
	if enabled {
		val = "true"
	}
	return s.pool.SetProjectConfig(projectID, workflowCleanupEnabledKey, val)
}

// GetSessionRetentionLimit returns the session retention limit for a project.
// Falls back to the global setting, then defaults to 1000.
func (s *GlobalSettingsService) GetSessionRetentionLimit(projectID string) (int, error) {
	val, err := s.pool.GetProjectConfig(projectID, sessionRetentionLimitKey)
	if err != nil {
		return 0, err
	}
	if val != "" {
		if parsed, parseErr := strconv.Atoi(val); parseErr == nil && parsed >= 10 {
			return parsed, nil
		}
	}
	globalVal, err := s.pool.GetConfig(sessionRetentionLimitKey)
	if err != nil {
		return 0, err
	}
	if globalVal != "" {
		if parsed, parseErr := strconv.Atoi(globalVal); parseErr == nil && parsed >= 10 {
			return parsed, nil
		}
	}
	return 1000, nil
}

// SetSessionRetentionLimit persists the session retention limit for a project.
// Returns an error if n < 10.
func (s *GlobalSettingsService) SetSessionRetentionLimit(projectID string, n int) error {
	if n < 10 {
		return fmt.Errorf("session_retention_limit must be >= 10")
	}
	return s.pool.SetProjectConfig(projectID, sessionRetentionLimitKey, strconv.Itoa(n))
}
