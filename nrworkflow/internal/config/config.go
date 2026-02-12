package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	DefaultConfigDir  = ".nrworkflow"
	DefaultConfigFile = "config.json"
	DefaultPort       = 6587
	// ProjectConfigPath is the relative path to project-specific config
	ProjectConfigPath = ".claude/nrworkflow/config.json"
)

// Config represents the global nrworkflow configuration
type Config struct {
	Server ServerConfig `json:"server"`
}

// ProjectConfig represents project-specific configuration from .claude/nrworkflow/config.json
type ProjectConfig struct {
	Project string                 `json:"project"`
	CLI     map[string]interface{} `json:"cli,omitempty"`
	Agents  map[string]interface{} `json:"agents,omitempty"`
}

// ProjectConfigResult contains the result of finding a project config
type ProjectConfigResult struct {
	Config     *ProjectConfig
	ConfigPath string
	ConfigDir  string
}

// ServerConfig contains server-specific settings
type ServerConfig struct {
	Port        int      `json:"port"`
	CORSOrigins []string `json:"cors_origins"`
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:        DefaultPort,
			CORSOrigins: []string{"http://localhost:5173"},
		},
	}
}

// GetConfigPath returns the path to the config file
func GetConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, DefaultConfigDir, DefaultConfigFile), nil
}

// Load loads the configuration from disk
func Load() (*Config, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default config if file doesn't exist
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply defaults for missing fields
	if cfg.Server.Port == 0 {
		cfg.Server.Port = DefaultPort
	}
	if len(cfg.Server.CORSOrigins) == 0 {
		cfg.Server.CORSOrigins = []string{"http://localhost:5173"}
	}

	return &cfg, nil
}

// Save saves the configuration to disk
func Save(cfg *Config) error {
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// FindProjectConfig searches upward from startDir for .claude/nrworkflow/config.json
// Returns the parsed config, the full path to the config file, and the directory containing .claude/
func FindProjectConfig(startDir string) (*ProjectConfigResult, error) {
	if startDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %w", err)
		}
		startDir = cwd
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(startDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Search upward from startDir
	current := absPath
	for {
		configPath := filepath.Join(current, ProjectConfigPath)
		if _, err := os.Stat(configPath); err == nil {
			// Found config file
			data, err := os.ReadFile(configPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read config file: %w", err)
			}

			var cfg ProjectConfig
			if err := json.Unmarshal(data, &cfg); err != nil {
				return nil, fmt.Errorf("failed to parse config file %s: %w", configPath, err)
			}

			return &ProjectConfigResult{
				Config:     &cfg,
				ConfigPath: configPath,
				ConfigDir:  current,
			}, nil
		}

		// Move to parent directory
		parent := filepath.Dir(current)
		if parent == current {
			// Reached root, config not found
			return nil, nil
		}
		current = parent
	}
}
