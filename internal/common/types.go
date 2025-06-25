// Package common provides shared types and utilities used across the application.
package common

import "github.com/tmc/langchaingo/llms"

// ToolInfo holds detailed information about a discovered tool
type ToolInfo struct {
	ServerName string
	Tool       *llms.Tool
}
