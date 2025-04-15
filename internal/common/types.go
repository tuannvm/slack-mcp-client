package common

// ToolInfo holds detailed information about a discovered tool
type ToolInfo struct {
	ServerName  string                 `json:"server_name"` // Added json tags for potential future use
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"` 
} 