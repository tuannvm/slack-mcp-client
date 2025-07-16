// Package security provides configuration loading utilities
package security

// LoadSecurityConfig loads and returns a security configuration with environment overrides
func LoadSecurityConfig() *SecurityConfig {
	config := DefaultSecurityConfig()
	config.LoadFromEnvironment()
	return config
}
