package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tuannvm/slack-mcp-client/internal/monitoring"
)

type ToolInfo struct {
	ServerName       string
	ToolName         string
	ToolDescription  string
	InputSchema      map[string]interface{}
	InputSchemaBytes []byte
	Client           MCPClientInterface
}

func (t *ToolInfo) Name() string {
	return t.ToolName
}

func (t *ToolInfo) Description() string {
	if t.InputSchemaBytes == nil {
		t.InputSchemaBytes, _ = json.Marshal(t.InputSchema)
	}
	return t.ToolDescription + "\n The input schema is: " + string(t.InputSchemaBytes)
}

func (t *ToolInfo) Call(ctx context.Context, input string) (string, error) {
	var args map[string]interface{}
	err := json.Unmarshal([]byte(input), &args)
	if err != nil {
		// Return parse error as a tool result so the LLM can see what happened
		// and retry with valid JSON, rather than crashing the agent loop.
		return fmt.Sprintf("[Tool Error] Invalid JSON input for %s: %v. Your Action Input must be a single-line valid JSON object with no trailing text.", t.Name(), err), nil
	}

	isError := "false"
	defer func() {
		monitoring.ToolInvocations.With(prometheus.Labels{
			monitoring.MetricLabelTool:   t.ToolName,
			monitoring.MetricLabelServer: t.ServerName,
			monitoring.MetricLabelError:  isError,
		}).Inc()
	}()

	res, err := t.Client.CallTool(ctx, t.Name(), args)
	if err != nil {
		isError = "true"
		// Return error as a tool result instead of a Go error so the LLM agent
		// can see what happened and retry or use a different approach, rather
		// than crashing the entire agent loop.
		return fmt.Sprintf("[Tool Error] %s failed: %v", t.Name(), err), nil
	}

	return res, nil
}
