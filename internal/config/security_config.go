// Package config provides security configuration extensions
package config

import (
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
	"github.com/tuannvm/slack-mcp-client/internal/security"
)

// ConfigWithSecurity extends the main Config with security configuration
type ConfigWithSecurity struct {
	*Config
	Security *security.SecurityConfig `json:"security,omitempty"`
}

// LoadConfigWithSecurity loads configuration including security settings
func LoadConfigWithSecurity(configFile string, logger *logging.Logger) (*ConfigWithSecurity, error) {
	// Load the main configuration
	mainConfig, err := LoadConfig(configFile, logger)
	if err != nil {
		return nil, err
	}

	// Load security configuration
	securityConfig := security.LoadSecurityConfig()

	// Log security configuration status
	if logger != nil {
		logger.InfoKV("Security configuration loaded",
			"enabled", securityConfig.Enabled,
			"strict_mode", securityConfig.StrictMode,
			"allowed_users_count", len(securityConfig.AllowedUsers),
			"allowed_channels_count", len(securityConfig.AllowedChannels),
			"admin_users_count", len(securityConfig.AdminUsers),
		)
	}

	return &ConfigWithSecurity{
		Config:   mainConfig,
		Security: securityConfig,
	}, nil
}
