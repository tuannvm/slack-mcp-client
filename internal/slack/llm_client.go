// Package slackbot implements the Slack integration for the MCP client
package slackbot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tuannvm/slack-mcp-client/internal/common"
	"github.com/tuannvm/slack-mcp-client/internal/config"
	"github.com/tuannvm/slack-mcp-client/internal/mcp"
)

// callLLM is a wrapper function that calls the appropriate LLM implementation
func (c *Client) callLLM(prompt, contextHistory string) (string, error) {
	// Try LangChain first regardless of configuration if it's available
	if hasLangChainTool(c.discoveredTools) {
		c.log.Printf("LangChain gateway available, using it for all LLM interactions")
		return c.callLangChainMCP(prompt, contextHistory)
	}

	// LangChain not available, fall back to configured provider
	switch c.cfg.LLMProvider {
	case config.ProviderLangChain:
		// If LangChain is explicitly configured but not available, log warning
		c.log.Printf("Warning: LangChain configured but not available as a tool, falling back to OpenAI")
		return c.callOpenAI(prompt, contextHistory)
	case config.ProviderOpenAI:
		return c.callOpenAI(prompt, contextHistory)
	default:
		c.log.Printf("Unknown LLM provider '%s', falling back to OpenAI", c.cfg.LLMProvider)
		return c.callOpenAI(prompt, contextHistory)
	}
}

// hasLangChainTool checks if the langchain tool is available in any server
func hasLangChainTool(tools map[string]common.ToolInfo) bool {
	for name := range tools {
		if name == "langchain" {
			return true
		}
	}
	return false
}

// callLangChainMCP sends a prompt to the LangChain handler via MCP
func (c *Client) callLangChainMCP(prompt, contextHistory string) (string, error) {
	c.log.Printf("Calling LangChain (via MCP) with prompt length: %d", len(prompt))

	// Find a client that has the LangChain tool
	var mcpClient *mcp.Client
	var serverName string

	for name, client := range c.mcpClients {
		// Check if this client has the LangChain tool registered
		if hasToolOnServer("langchain", name, c.discoveredTools) {
			mcpClient = client
			serverName = name
			break
		}
	}

	if mcpClient == nil {
		return "", fmt.Errorf("no MCP client has the 'langchain' tool available")
	}

	c.log.Printf("Using MCP server '%s' for LangChain call", serverName)

	// Prepare messages array
	var messages []map[string]interface{}

	// Add system prompt with tool info if available
	systemPrompt := c.generateToolPrompt()
	if systemPrompt != "" {
		messages = append(messages, map[string]interface{}{
			"role":    "system",
			"content": systemPrompt,
		})
	}

	// Add conversation context if provided
	if contextHistory != "" {
		messages = append(messages, map[string]interface{}{
			"role":    "system",
			"content": "Previous conversation: " + contextHistory,
		})
	}

	// Add the current prompt
	messages = append(messages, map[string]interface{}{
		"role":    "user",
		"content": prompt,
	})

	// Prepare the arguments for the tool call
	args := map[string]interface{}{
		"model":    c.cfg.OpenAIModelName,
		"messages": messages,
	}

	// For certain models, add specific parameters
	if c.cfg.OpenAIModelName != "" {
		if strings.Contains(c.cfg.OpenAIModelName, "gpt-4o") {
			args["temperature"] = 0.7
			args["max_tokens"] = 2048
		}
	}

	// Create the context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Call the langchain tool
	responseText, err := mcpClient.CallTool(ctx, "langchain", args)
	if err != nil {
		return "", fmt.Errorf("LangChain call failed: %w", err)
	}

	// Check for empty response
	if responseText == "" {
		return "", fmt.Errorf("received empty response from LangChain")
	}

	c.log.Printf("Received LangChain response of length %d", len(responseText))
	return responseText, nil
}

// hasToolOnServer checks if a specific tool is available on a server
func hasToolOnServer(toolName, serverName string, tools map[string]common.ToolInfo) bool {
	for name, toolInfo := range tools {
		if name == toolName && toolInfo.ServerName == serverName {
			return true
		}
	}
	return false
}
