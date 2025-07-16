// Package security provides access control and security features for the Slack MCP client
package security

import (
	"os"
	"strings"
)

// SecurityConfig defines the security configuration for the application
type SecurityConfig struct {
	Enabled          bool     `json:"enabled"`           // Enable/disable security
	StrictMode       bool     `json:"strict_mode"`       // Require both user AND channel whitelisting
	AllowedUsers     []string `json:"allowed_users"`     // List of allowed user IDs
	AllowedChannels  []string `json:"allowed_channels"`  // List of allowed channel IDs
	AdminUsers       []string `json:"admin_users"`       // Admin users (bypass channel restrictions)
	RejectionMessage string   `json:"rejection_message"` // Custom rejection message
	LogUnauthorized  bool     `json:"log_unauthorized"`  // Log unauthorized attempts
}

// DefaultSecurityConfig returns a default security configuration
func DefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		Enabled:          false,
		StrictMode:       false,
		AllowedUsers:     []string{},
		AllowedChannels:  []string{},
		AdminUsers:       []string{},
		RejectionMessage: "I'm sorry, but I don't have permission to respond in this context. Please contact your administrator if you believe this is an error.",
		LogUnauthorized:  true,
	}
}

// LoadFromEnvironment loads security configuration from environment variables
func (sc *SecurityConfig) LoadFromEnvironment() {
	// SECURITY_ENABLED
	if enabled := os.Getenv("SECURITY_ENABLED"); enabled != "" {
		sc.Enabled = strings.ToLower(enabled) == "true"
	}

	// SECURITY_STRICT_MODE
	if strictMode := os.Getenv("SECURITY_STRICT_MODE"); strictMode != "" {
		sc.StrictMode = strings.ToLower(strictMode) == "true"
	}

	// SECURITY_ALLOWED_USERS
	if allowedUsers := os.Getenv("SECURITY_ALLOWED_USERS"); allowedUsers != "" {
		sc.AllowedUsers = parseCommaSeparated(allowedUsers)
	}

	// SECURITY_ALLOWED_CHANNELS
	if allowedChannels := os.Getenv("SECURITY_ALLOWED_CHANNELS"); allowedChannels != "" {
		sc.AllowedChannels = parseCommaSeparated(allowedChannels)
	}

	// SECURITY_ADMIN_USERS
	if adminUsers := os.Getenv("SECURITY_ADMIN_USERS"); adminUsers != "" {
		sc.AdminUsers = parseCommaSeparated(adminUsers)
	}

	// SECURITY_REJECTION_MESSAGE
	if rejectionMessage := os.Getenv("SECURITY_REJECTION_MESSAGE"); rejectionMessage != "" {
		sc.RejectionMessage = rejectionMessage
	}

	// SECURITY_LOG_UNAUTHORIZED
	if logUnauthorized := os.Getenv("SECURITY_LOG_UNAUTHORIZED"); logUnauthorized != "" {
		sc.LogUnauthorized = strings.ToLower(logUnauthorized) == "true"
	}
}

// parseCommaSeparated parses a comma-separated string into a slice of trimmed strings
func parseCommaSeparated(input string) []string {
	if input == "" {
		return []string{}
	}

	parts := strings.Split(input, ",")
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

// IsUserAllowed checks if a user ID is in the allowed users list
func (sc *SecurityConfig) IsUserAllowed(userID string) bool {
	return contains(sc.AllowedUsers, userID)
}

// IsChannelAllowed checks if a channel ID is in the allowed channels list
func (sc *SecurityConfig) IsChannelAllowed(channelID string) bool {
	return contains(sc.AllowedChannels, channelID)
}

// IsAdminUser checks if a user ID is in the admin users list
func (sc *SecurityConfig) IsAdminUser(userID string) bool {
	return contains(sc.AdminUsers, userID)
}

// contains checks if a slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
