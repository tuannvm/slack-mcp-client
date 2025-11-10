package config

import (
	"os"
	"testing"
)

func TestSecurityDefaults(t *testing.T) {
	c := &Config{}
	c.applySecurityDefaults()

	// When security is disabled, rejection message should remain empty
	if c.Security.RejectionMessage != "" {
		t.Errorf("Expected empty rejection message when security disabled, got: %s", c.Security.RejectionMessage)
	}

	// When security is enabled, default rejection message should be set
	c.Security.Enabled = true
	c.applySecurityDefaults()

	expectedMessage := "I'm sorry, but I don't have permission to respond in this context. Please contact the app administrator if you believe this is an error."
	if c.Security.RejectionMessage != expectedMessage {
		t.Errorf("Expected default rejection message, got: %s", c.Security.RejectionMessage)
	}

	// Custom message should not be overridden
	customMessage := "Custom rejection message"
	c.Security.RejectionMessage = customMessage
	c.applySecurityDefaults()

	if c.Security.RejectionMessage != customMessage {
		t.Errorf("Expected custom message to be preserved, got: %s", c.Security.RejectionMessage)
	}
}

func TestSecurityEnvironmentVariables(t *testing.T) {
	// Save original environment
	originalVars := map[string]string{
		"SECURITY_ENABLED":           os.Getenv("SECURITY_ENABLED"),
		"SECURITY_STRICT_MODE":       os.Getenv("SECURITY_STRICT_MODE"),
		"SECURITY_LOG_UNAUTHORIZED":  os.Getenv("SECURITY_LOG_UNAUTHORIZED"),
		"SECURITY_ALLOWED_USERS":     os.Getenv("SECURITY_ALLOWED_USERS"),
		"SECURITY_ALLOWED_CHANNELS":  os.Getenv("SECURITY_ALLOWED_CHANNELS"),
		"SECURITY_ADMIN_USERS":       os.Getenv("SECURITY_ADMIN_USERS"),
		"SECURITY_REJECTION_MESSAGE": os.Getenv("SECURITY_REJECTION_MESSAGE"),
	}

	// Clean environment
	for key := range originalVars {
		_ = os.Unsetenv(key)
	}

	// Restore environment after test
	defer func() {
		for key, value := range originalVars {
			if value != "" {
				_ = os.Setenv(key, value)
			} else {
				_ = os.Unsetenv(key)
			}
		}
	}()

	tests := []struct {
		name     string
		envVars  map[string]string
		expected SecurityConfig
	}{
		{
			name: "Basic security enabled - should set default rejection message",
			envVars: map[string]string{
				"SECURITY_ENABLED": "true",
			},
			expected: SecurityConfig{
				Enabled:          true,
				RejectionMessage: "I'm sorry, but I don't have permission to respond in this context. Please contact the app administrator if you believe this is an error.",
			},
		},
		{
			name: "Strict mode enabled",
			envVars: map[string]string{
				"SECURITY_ENABLED":     "true",
				"SECURITY_STRICT_MODE": "true",
			},
			expected: SecurityConfig{
				Enabled:          true,
				StrictMode:       true,
				RejectionMessage: "I'm sorry, but I don't have permission to respond in this context. Please contact the app administrator if you believe this is an error.",
			},
		},
		{
			name: "Log unauthorized enabled",
			envVars: map[string]string{
				"SECURITY_ENABLED":          "true",
				"SECURITY_LOG_UNAUTHORIZED": "true",
			},
			expected: SecurityConfig{
				Enabled:          true,
				LogUnauthorized:  true,
				RejectionMessage: "I'm sorry, but I don't have permission to respond in this context. Please contact the app administrator if you believe this is an error.",
			},
		},
		{
			name: "User and channel lists",
			envVars: map[string]string{
				"SECURITY_ENABLED":          "true",
				"SECURITY_ALLOWED_USERS":    "U123456789, U987654321 ,U555555555",
				"SECURITY_ALLOWED_CHANNELS": "C123456789,C987654321, C555555555 ",
				"SECURITY_ADMIN_USERS":      " A123456789 , A987654321",
			},
			expected: SecurityConfig{
				Enabled:          true,
				AllowedUsers:     []string{"U123456789", "U987654321", "U555555555"},
				AllowedChannels:  []string{"C123456789", "C987654321", "C555555555"},
				AdminUsers:       []string{"A123456789", "A987654321"},
				RejectionMessage: "I'm sorry, but I don't have permission to respond in this context. Please contact the app administrator if you believe this is an error.",
			},
		},
		{
			name: "Malformed input with empty strings - should filter them out",
			envVars: map[string]string{
				"SECURITY_ENABLED":          "true",
				"SECURITY_ALLOWED_USERS":    "U123,,U456,  ,U789",
				"SECURITY_ALLOWED_CHANNELS": "C123,,,C456, , ,C789",
				"SECURITY_ADMIN_USERS":      "A123, , ,A456,,",
			},
			expected: SecurityConfig{
				Enabled:          true,
				AllowedUsers:     []string{"U123", "U456", "U789"},
				AllowedChannels:  []string{"C123", "C456", "C789"},
				AdminUsers:       []string{"A123", "A456"},
				RejectionMessage: "I'm sorry, but I don't have permission to respond in this context. Please contact the app administrator if you believe this is an error.",
			},
		},
		{
			name: "Custom rejection message",
			envVars: map[string]string{
				"SECURITY_ENABLED":           "true",
				"SECURITY_REJECTION_MESSAGE": "Access denied. Contact admin.",
			},
			expected: SecurityConfig{
				Enabled:          true,
				RejectionMessage: "Access denied. Contact admin.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for key, value := range tt.envVars {
				_ = os.Setenv(key, value)
			}

			// Clean up after each test
			defer func() {
				for key := range tt.envVars {
					_ = os.Unsetenv(key)
				}
			}()

			c := &Config{}
			c.ApplyEnvironmentVariables()

			// Check boolean fields
			if c.Security.Enabled != tt.expected.Enabled {
				t.Errorf("Expected Enabled=%v, got=%v", tt.expected.Enabled, c.Security.Enabled)
			}
			if c.Security.StrictMode != tt.expected.StrictMode {
				t.Errorf("Expected StrictMode=%v, got=%v", tt.expected.StrictMode, c.Security.StrictMode)
			}
			if c.Security.LogUnauthorized != tt.expected.LogUnauthorized {
				t.Errorf("Expected LogUnauthorized=%v, got=%v", tt.expected.LogUnauthorized, c.Security.LogUnauthorized)
			}

			// Check string field
			if c.Security.RejectionMessage != tt.expected.RejectionMessage {
				t.Errorf("Expected RejectionMessage=%s, got=%s", tt.expected.RejectionMessage, c.Security.RejectionMessage)
			}

			// Check slice fields
			if !stringSlicesEqual(c.Security.AllowedUsers, tt.expected.AllowedUsers) {
				t.Errorf("Expected AllowedUsers=%v, got=%v", tt.expected.AllowedUsers, c.Security.AllowedUsers)
			}
			if !stringSlicesEqual(c.Security.AllowedChannels, tt.expected.AllowedChannels) {
				t.Errorf("Expected AllowedChannels=%v, got=%v", tt.expected.AllowedChannels, c.Security.AllowedChannels)
			}
			if !stringSlicesEqual(c.Security.AdminUsers, tt.expected.AdminUsers) {
				t.Errorf("Expected AdminUsers=%v, got=%v", tt.expected.AdminUsers, c.Security.AdminUsers)
			}
		})
	}
}

func TestValidateAccess(t *testing.T) {
	tests := []struct {
		name      string
		config    SecurityConfig
		userID    string
		channelID string
		expected  SecurityResult
	}{
		{
			name:      "Security disabled - should allow access",
			config:    SecurityConfig{Enabled: false},
			userID:    "U123456789",
			channelID: "C123456789",
			expected: SecurityResult{
				Allowed: true,
				Reason:  "Security disabled",
			},
		},
		{
			name: "Admin user - should allow access regardless of channel",
			config: SecurityConfig{
				Enabled:         true,
				AllowedUsers:    []string{"U123456789"},
				AllowedChannels: []string{"C123456789"},
				AdminUsers:      []string{"A123456789"},
			},
			userID:    "A123456789",
			channelID: "C999999999", // Not in allowed channels
			expected: SecurityResult{
				Allowed: true,
				Reason:  "Admin user access",
			},
		},
		{
			name: "Flexible mode - user whitelisted",
			config: SecurityConfig{
				Enabled:         true,
				StrictMode:      false,
				AllowedUsers:    []string{"U123456789"},
				AllowedChannels: []string{"C123456789"},
			},
			userID:    "U123456789",
			channelID: "C999999999", // Not in allowed channels
			expected: SecurityResult{
				Allowed: true,
				Reason:  "User whitelisted",
			},
		},
		{
			name: "Flexible mode - channel whitelisted",
			config: SecurityConfig{
				Enabled:         true,
				StrictMode:      false,
				AllowedUsers:    []string{"U123456789"},
				AllowedChannels: []string{"C123456789"},
			},
			userID:    "U999999999", // Not in allowed users
			channelID: "C123456789",
			expected: SecurityResult{
				Allowed: true,
				Reason:  "Channel whitelisted",
			},
		},
		{
			name: "Flexible mode - both user and channel whitelisted",
			config: SecurityConfig{
				Enabled:         true,
				StrictMode:      false,
				AllowedUsers:    []string{"U123456789"},
				AllowedChannels: []string{"C123456789"},
			},
			userID:    "U123456789",
			channelID: "C123456789",
			expected: SecurityResult{
				Allowed: true,
				Reason:  "User and channel both whitelisted",
			},
		},
		{
			name: "Flexible mode - neither user nor channel whitelisted",
			config: SecurityConfig{
				Enabled:         true,
				StrictMode:      false,
				AllowedUsers:    []string{"U123456789"},
				AllowedChannels: []string{"C123456789"},
			},
			userID:    "U999999999",
			channelID: "C999999999",
			expected: SecurityResult{
				Allowed: false,
				Reason:  "Neither user nor channel whitelisted",
			},
		},
		{
			name: "Strict mode - both user and channel whitelisted",
			config: SecurityConfig{
				Enabled:         true,
				StrictMode:      true,
				AllowedUsers:    []string{"U123456789"},
				AllowedChannels: []string{"C123456789"},
			},
			userID:    "U123456789",
			channelID: "C123456789",
			expected: SecurityResult{
				Allowed: true,
				Reason:  "User and channel both whitelisted (strict mode)",
			},
		},
		{
			name: "Strict mode - user whitelisted but channel not",
			config: SecurityConfig{
				Enabled:         true,
				StrictMode:      true,
				AllowedUsers:    []string{"U123456789"},
				AllowedChannels: []string{"C123456789"},
			},
			userID:    "U123456789",
			channelID: "C999999999",
			expected: SecurityResult{
				Allowed: false,
				Reason:  "Channel not whitelisted (strict mode)",
			},
		},
		{
			name: "Strict mode - channel whitelisted but user not",
			config: SecurityConfig{
				Enabled:         true,
				StrictMode:      true,
				AllowedUsers:    []string{"U123456789"},
				AllowedChannels: []string{"C123456789"},
			},
			userID:    "U999999999",
			channelID: "C123456789",
			expected: SecurityResult{
				Allowed: false,
				Reason:  "User not whitelisted (strict mode)",
			},
		},
		{
			name: "Strict mode - neither user nor channel whitelisted",
			config: SecurityConfig{
				Enabled:         true,
				StrictMode:      true,
				AllowedUsers:    []string{"U123456789"},
				AllowedChannels: []string{"C123456789"},
			},
			userID:    "U999999999",
			channelID: "C999999999",
			expected: SecurityResult{
				Allowed: false,
				Reason:  "Neither user nor channel whitelisted (strict mode)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{Security: tt.config}
			result := c.ValidateAccess(tt.userID, tt.channelID)

			if result.Allowed != tt.expected.Allowed {
				t.Errorf("Expected Allowed=%v, got=%v", tt.expected.Allowed, result.Allowed)
			}
			if result.Reason != tt.expected.Reason {
				t.Errorf("Expected Reason=%s, got=%s", tt.expected.Reason, result.Reason)
			}
		})
	}
}

func TestHelperMethods(t *testing.T) {
	c := &Config{
		Security: SecurityConfig{
			AllowedUsers:    []string{"U123456789", "U987654321"},
			AllowedChannels: []string{"C123456789", "C987654321"},
			AdminUsers:      []string{"A123456789", "A987654321"},
		},
	}

	// Test isUserAllowed
	if !c.isUserAllowed("U123456789") {
		t.Error("Expected U123456789 to be allowed")
	}
	if c.isUserAllowed("U999999999") {
		t.Error("Expected U999999999 to not be allowed")
	}

	// Test isChannelAllowed
	if !c.isChannelAllowed("C123456789") {
		t.Error("Expected C123456789 to be allowed")
	}
	if c.isChannelAllowed("C999999999") {
		t.Error("Expected C999999999 to not be allowed")
	}

	// Test isAdminUser
	if !c.isAdminUser("A123456789") {
		t.Error("Expected A123456789 to be admin")
	}
	if c.isAdminUser("A999999999") {
		t.Error("Expected A999999999 to not be admin")
	}
}

// Helper function to compare string slices
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}
