// Package rag provides a RAG client wrapper for MCP integration
package rag

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// MCPClientInterface defines the interface that MCP clients should implement
type MCPClientInterface interface {
	CallTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error)
}

// Client wraps SimpleRAG to implement the MCPClientInterface
// This allows the LLM-MCP bridge to treat RAG as a regular MCP tool
type Client struct {
	rag     *SimpleRAG
	maxDocs int // Maximum documents to return in a single call
}

// NewClient creates a new RAG client wrapper
func NewClient(ragDatabase string) *Client {
	return &Client{
		rag:     NewSimpleRAG(ragDatabase),
		maxDocs: 10, // Reasonable default
	}
}

// CallTool implements the MCPClientInterface for RAG tools
func (c *Client) CallTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
	// Validate context
	if ctx == nil {
		return "", fmt.Errorf("context cannot be nil")
	}

	// Validate arguments
	if args == nil {
		return "", fmt.Errorf("arguments cannot be nil")
	}

	switch toolName {
	case "rag_search":
		return c.handleRAGSearch(ctx, args)
	case "rag_ingest":
		return c.handleRAGIngest(ctx, args)
	case "rag_stats":
		return c.handleRAGStats(ctx, args)
	default:
		return "", fmt.Errorf("unknown RAG tool: %s. Available tools: rag_search, rag_ingest, rag_stats", toolName)
	}
}

// handleRAGSearch processes search requests
func (c *Client) handleRAGSearch(ctx context.Context, args map[string]interface{}) (string, error) {
	// Extract and validate query parameter
	query, err := c.extractStringParam(args, "query", true)
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query cannot be empty")
	}

	// Extract limit parameter with validation
	limit, err := c.extractIntParam(args, "limit", false, 3)
	if err != nil {
		return "", err
	}

	// Validate limit bounds
	if limit < 1 {
		limit = 1
	} else if limit > c.maxDocs {
		limit = c.maxDocs
	}

	// Perform search with context cancellation support
	resultChan := make(chan searchResult, 1)
	go func() {
		docs := c.rag.Search(query, limit)
		resultChan <- searchResult{docs: docs}
	}()

	select {
	case <-ctx.Done():
		return "", fmt.Errorf("search cancelled: %w", ctx.Err())
	case result := <-resultChan:
		return c.formatSearchResults(result.docs, query), nil
	}
}

// handleRAGIngest processes document ingestion requests
func (c *Client) handleRAGIngest(ctx context.Context, args map[string]interface{}) (string, error) {
	filePath, err := c.extractStringParam(args, "file_path", true)
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(filePath) == "" {
		return "", fmt.Errorf("file_path cannot be empty")
	}

	// Check if it's a directory or file
	if isDirectory, _ := c.extractBoolParam(args, "is_directory", false, false); isDirectory {
		count, err := c.rag.IngestDirectory(filePath)
		if err != nil {
			return "", fmt.Errorf("failed to ingest directory %s: %w", filePath, err)
		}
		return fmt.Sprintf("Successfully ingested %d PDF files from directory: %s", count, filePath), nil
	} else {
		err := c.rag.IngestPDF(filePath)
		if err != nil {
			return "", fmt.Errorf("failed to ingest PDF %s: %w", filePath, err)
		}
		return fmt.Sprintf("Successfully ingested PDF: %s", filePath), nil
	}
}

// handleRAGStats returns statistics about the knowledge base
func (c *Client) handleRAGStats(ctx context.Context, args map[string]interface{}) (string, error) {
	count := c.rag.GetDocumentCount()
	return fmt.Sprintf("Knowledge base contains %d document chunks", count), nil
}

type searchResult struct {
	docs []Document
}

// formatSearchResults formats search results for display
func (c *Client) formatSearchResults(docs []Document, query string) string {
	if len(docs) == 0 {
		return fmt.Sprintf("No relevant context found for query: '%s'", query)
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Found %d relevant context(s) for '%s':\n\n", len(docs), query))

	for i, doc := range docs {
		result.WriteString(fmt.Sprintf("--- Context %d ---\n", i+1))

		// Add source information if available
		if fileName, ok := doc.Metadata["file_name"]; ok {
			result.WriteString(fmt.Sprintf("Source: %s", fileName))
			if chunkIndex, hasChunk := doc.Metadata["chunk_index"]; hasChunk {
				result.WriteString(fmt.Sprintf(" (chunk %s)", chunkIndex))
			}
			result.WriteString("\n")
		}

		// Add content with reasonable truncation
		content := strings.TrimSpace(doc.Content)
		if len(content) > 800 {
			content = content[:800] + "..."
		}
		result.WriteString(fmt.Sprintf("Content: %s\n\n", content))
	}

	return result.String()
}

// extractStringParam safely extracts a string parameter
func (c *Client) extractStringParam(args map[string]interface{}, key string, required bool) (string, error) {
	value, exists := args[key]
	if !exists {
		if required {
			return "", fmt.Errorf("required parameter '%s' not found", key)
		}
		return "", nil
	}

	switch v := value.(type) {
	case string:
		return v, nil
	case nil:
		if required {
			return "", fmt.Errorf("parameter '%s' cannot be nil", key)
		}
		return "", nil
	default:
		return "", fmt.Errorf("parameter '%s' must be a string, got %T", key, value)
	}
}

// extractIntParam safely extracts an integer parameter
func (c *Client) extractIntParam(args map[string]interface{}, key string, required bool, defaultValue int) (int, error) {
	value, exists := args[key]
	if !exists {
		if required {
			return 0, fmt.Errorf("required parameter '%s' not found", key)
		}
		return defaultValue, nil
	}

	switch v := value.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	case string:
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("parameter '%s' must be a valid integer, got '%s'", key, v)
		}
		return parsed, nil
	case nil:
		if required {
			return 0, fmt.Errorf("parameter '%s' cannot be nil", key)
		}
		return defaultValue, nil
	default:
		return 0, fmt.Errorf("parameter '%s' must be an integer, got %T", key, value)
	}
}

// extractBoolParam safely extracts a boolean parameter
func (c *Client) extractBoolParam(args map[string]interface{}, key string, required bool, defaultValue bool) (bool, error) {
	value, exists := args[key]
	if !exists {
		if required {
			return false, fmt.Errorf("required parameter '%s' not found", key)
		}
		return defaultValue, nil
	}

	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			return false, fmt.Errorf("parameter '%s' must be a valid boolean, got '%s'", key, v)
		}
		return parsed, nil
	case nil:
		if required {
			return false, fmt.Errorf("parameter '%s' cannot be nil", key)
		}
		return defaultValue, nil
	default:
		return false, fmt.Errorf("parameter '%s' must be a boolean, got %T", key, value)
	}
}

// GetRAG returns the underlying SimpleRAG instance for direct access
func (c *Client) GetRAG() *SimpleRAG {
	return c.rag
}

// SetMaxDocs configures the maximum number of documents to return in search results
func (c *Client) SetMaxDocs(max int) {
	if max > 0 {
		c.maxDocs = max
	}
}
