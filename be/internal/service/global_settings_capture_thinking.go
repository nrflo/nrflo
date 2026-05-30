package service

const captureThinkingEnabledKey = "capture_thinking_enabled"

// GetCaptureThinkingEnabled returns whether capture-thinking is enabled for a project.
// Project config overrides global; defaults to false.
func (s *GlobalSettingsService) GetCaptureThinkingEnabled(projectID string) (bool, error) {
	projectVal, err := s.pool.GetProjectConfig(projectID, captureThinkingEnabledKey)
	if err != nil {
		return false, err
	}
	if projectVal != "" {
		return projectVal == "true", nil
	}
	globalVal, err := s.pool.GetConfig(captureThinkingEnabledKey)
	if err != nil {
		return false, err
	}
	return globalVal == "true", nil
}

// SetCaptureThinkingEnabled persists the capture-thinking enabled flag globally.
func (s *GlobalSettingsService) SetCaptureThinkingEnabled(enabled bool) error {
	val := "false"
	if enabled {
		val = "true"
	}
	return s.pool.SetConfig(captureThinkingEnabledKey, val)
}

// SetCaptureThinkingEnabledForProject sets the per-project capture-thinking override.
// A nil enabled clears the override (inherits global).
func (s *GlobalSettingsService) SetCaptureThinkingEnabledForProject(projectID string, enabled *bool) error {
	if enabled == nil {
		return s.pool.SetProjectConfig(projectID, captureThinkingEnabledKey, "")
	}
	val := "false"
	if *enabled {
		val = "true"
	}
	return s.pool.SetProjectConfig(projectID, captureThinkingEnabledKey, val)
}
