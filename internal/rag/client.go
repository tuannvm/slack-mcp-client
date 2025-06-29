// Package rag provides a RAG client wrapper for MCP integration
package rag

import (
	"context"
	"fmt"
)

// Client wraps SimpleRAG to implement the MCPClientInterface
// This allows the LLM-MCP bridge to treat RAG as a regular MCP tool
type Client struct {
	rag *SimpleRAG
}

// NewClient creates a new RAG client wrapper
func NewClient(ragDatabase string) *Client {
	return &Client{
		rag: NewSimpleRAG(ragDatabase),
	}
}

// CallTool implements the MCPClientInterface for RAG tools
func (c *Client) CallTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
	switch toolName {
	case "rag_search":
		// Extract query parameter
		query, ok := args["query"].(string)
		if !ok {
			return "", fmt.Errorf("query parameter required and must be a string")
		}

		// Extract limit parameter with default
		limit := 3
		if limitInterface, exists := args["limit"]; exists {
			switch v := limitInterface.(type) {
			case int:
				limit = v
			case float64:
				limit = int(v)
			case string:
				// Try to parse string as int
				if parsedLimit, err := fmt.Sscanf(v, "%d", &limit); err != nil || parsedLimit != 1 {
					limit = 3 // fallback to default
				}
			}
		}

		// Perform search
		docs := c.rag.Search(query, limit)

		// Build response
		if len(docs) == 0 {
			return "No relevant context found in knowledge base.", nil
		}

		result := "Found relevant context:\n\n"
		for i, doc := range docs {
			result += fmt.Sprintf("Context %d", i+1)
			if fileName, ok := doc.Metadata["file_name"]; ok {
				result += fmt.Sprintf(" (from %s)", fileName)
			}
			result += ":\n"
			result += doc.Content + "\n\n"
		}

		return result, nil

	default:
		return "", fmt.Errorf("unknown RAG tool: %s", toolName)
	}
}

// GetRAG returns the underlying SimpleRAG instance for direct access
func (c *Client) GetRAG() *SimpleRAG {
	return c.rag
}
