// Package slackbot provides a unified interface for Slack client operations
package slackbot

import "github.com/tuannvm/slack-mcp-client/internal/security"

// SlackRunner defines the interface for a Slack client that can run and handle security
type SlackRunner interface {
	Run() error
	UpdateSecurityConfig(securityConfig *security.SecurityConfig)
	GetSecurityConfig() security.SecurityConfig
	IsSecurityEnabled() bool
}
