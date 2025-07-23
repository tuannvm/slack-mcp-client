// Package security provides access control and security features for the Slack MCP client
package security

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
