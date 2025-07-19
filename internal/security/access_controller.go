// Package security provides access control functionality
package security

import (
	"fmt"

	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
)

// AccessDecision represents the result of an access control check
type AccessDecision struct {
	Allowed bool   // Whether access is allowed
	Reason  string // Human-readable reason for the decision
}

// AccessController handles access control decisions based on security configuration
type AccessController struct {
	config *SecurityConfig
	logger *logging.Logger
}

// NewAccessController creates a new AccessController instance
func NewAccessController(config *SecurityConfig, logger *logging.Logger) *AccessController {
	return &AccessController{
		config: config,
		logger: logger,
	}
}

// CheckAccess determines if a user has access to interact in a specific channel
func (ac *AccessController) CheckAccess(userID, channelID string) *AccessDecision {
	// If security is disabled, allow all access
	if !ac.config.Enabled {
		return &AccessDecision{
			Allowed: true,
			Reason:  "Security disabled",
		}
	}

	// Check if user is an admin (admins bypass channel restrictions)
	isAdmin := ac.config.IsAdminUser(userID)
	if isAdmin {
		decision := &AccessDecision{
			Allowed: true,
			Reason:  "Admin user access",
		}
		ac.logAccessAttempt(userID, channelID, decision)
		return decision
	}

	// Check user and channel permissions
	userAllowed := ac.config.IsUserAllowed(userID)
	channelAllowed := ac.config.IsChannelAllowed(channelID)

	var decision *AccessDecision

	if ac.config.StrictMode {
		// Strict mode: Both user AND channel must be allowed
		if userAllowed && channelAllowed {
			decision = &AccessDecision{
				Allowed: true,
				Reason:  "User and channel both whitelisted (strict mode)",
			}
		} else {
			reason := ac.buildRejectionReason(userAllowed, channelAllowed, true)
			decision = &AccessDecision{
				Allowed: false,
				Reason:  reason,
			}
		}
	} else {
		// Flexible mode: Either user OR channel must be allowed
		if userAllowed || channelAllowed {
			var reason string
			if userAllowed && channelAllowed {
				reason = "User and channel both whitelisted"
			} else if userAllowed {
				reason = "User whitelisted"
			} else {
				reason = "Channel whitelisted"
			}
			decision = &AccessDecision{
				Allowed: true,
				Reason:  reason,
			}
		} else {
			decision = &AccessDecision{
				Allowed: false,
				Reason:  "Neither user nor channel whitelisted",
			}
		}
	}

	// Log the access attempt
	ac.logAccessAttempt(userID, channelID, decision)

	return decision
}

// buildRejectionReason creates a detailed reason for access rejection
func (ac *AccessController) buildRejectionReason(userAllowed, channelAllowed, strictMode bool) string {
	if strictMode {
		if !userAllowed && !channelAllowed {
			return "User and channel not whitelisted (strict mode)"
		} else if !userAllowed {
			return "User not whitelisted (strict mode)"
		} else {
			return "Channel not whitelisted (strict mode)"
		}
	}
	return "Neither user nor channel whitelisted"
}

// logAccessAttempt logs the access attempt with relevant context
func (ac *AccessController) logAccessAttempt(userID, channelID string, decision *AccessDecision) {
	if !ac.config.LogUnauthorized && !decision.Allowed {
		// Skip logging unauthorized attempts if disabled
		return
	}

	logMessage := fmt.Sprintf("Access %s", map[bool]string{true: "granted", false: "denied"}[decision.Allowed])

	if decision.Allowed {
		ac.logger.InfoKV(
			logMessage,
			"user_id", userID,
			"channel_id", channelID,
			"allowed", decision.Allowed,
			"reason", decision.Reason,
			"security_enabled", ac.config.Enabled,
			"strict_mode", ac.config.StrictMode,
		)
	} else {
		ac.logger.WarnKV(
			logMessage,
			"user_id", userID,
			"channel_id", channelID,
			"allowed", decision.Allowed,
			"reason", decision.Reason,
			"security_enabled", ac.config.Enabled,
			"strict_mode", ac.config.StrictMode,
		)
	}
}

// GetRejectionMessage returns the configured rejection message
func (ac *AccessController) GetRejectionMessage() string {
	return ac.config.RejectionMessage
}

// UpdateConfig updates the security configuration
func (ac *AccessController) UpdateConfig(config *SecurityConfig) {
	ac.config = config
	ac.logger.InfoKV("Security configuration updated",
		"enabled", config.Enabled,
		"strict_mode", config.StrictMode,
		"allowed_users_count", len(config.AllowedUsers),
		"allowed_channels_count", len(config.AllowedChannels),
		"admin_users_count", len(config.AdminUsers),
	)
}

// GetConfig returns the current security configuration (read-only)
func (ac *AccessController) GetConfig() SecurityConfig {
	return *ac.config
}

// IsSecurityEnabled returns whether security is currently enabled
func (ac *AccessController) IsSecurityEnabled() bool {
	return ac.config.Enabled
}
