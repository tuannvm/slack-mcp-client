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

// Client wraps RAG providers to implement the MCPClientInterface
// This allows the LLM-MCP bridge to treat RAG as a regular MCP tool
type Client struct {
	provider RAGProvider            // Can be SimpleRAG, VectorProviderAdapter, etc.
	factory  *RAGFactory            // Factory for creating providers
	maxDocs  int                    // Maximum documents to return in a single call
	config   map[string]interface{} // Configuration for provider
}

// NewClient creates a new RAG client wrapper with default simple provider
func NewClient(ragDatabase string) *Client {
	factory := NewRAGFactory("simple", ragDatabase)
	provider, _ := factory.CreateProvider("simple", nil)

	return &Client{
		provider: provider,
		factory:  factory,
		maxDocs:  10, // Reasonable default
		config:   make(map[string]interface{}),
	}
}

// NewClientWithProvider creates a new RAG client with specified provider
func NewClientWithProvider(providerType string, config map[string]interface{}) (*Client, error) {
	// Extract database path from config if available
	dbPath := "./knowledge.json"
	if path, ok := config["database_path"].(string); ok {
		dbPath = path
	}

	factory := NewRAGFactory(providerType, dbPath)
	provider, err := factory.CreateProvider(providerType, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}

	return &Client{
		provider: provider,
		factory:  factory,
		maxDocs:  10,
		config:   config,
	}, nil
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

	// Check if provider is a VectorProviderAdapter to use vector search
	if adapter, ok := c.provider.(*VectorProviderAdapter); ok {
		// Use vector search
		results, err := adapter.GetProvider().Search(ctx, query, SearchOptions{
			Limit: limit,
		})
		if err != nil {
			return "", fmt.Errorf("vector search failed: %w", err)
		}
		return c.formatVectorSearchResults(results, query), nil
	}

	// Fall back to traditional similarity search for other providers
	resultChan := make(chan searchResult, 1)
	go func() {
		// Use the RAGProvider interface - fall back to SimilaritySearch
		schemaResults, err := c.provider.SimilaritySearch(ctx, query, limit)
		var docs []Document
		if err == nil {
			// Convert schema.Document to our Document type
			docs = make([]Document, len(schemaResults))
			for i, schemaDoc := range schemaResults {
				metadata := make(map[string]string)
				for k, v := range schemaDoc.Metadata {
					if str, ok := v.(string); ok {
						metadata[k] = str
					} else {
						metadata[k] = fmt.Sprintf("%v", v)
					}
				}
				docs[i] = Document{
					Content:  schemaDoc.PageContent,
					Metadata: metadata,
				}
			}
		}
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

	// Check if provider is a VectorProviderAdapter
	if adapter, ok := c.provider.(*VectorProviderAdapter); ok {
		// Use vector provider for ingestion
		provider := adapter.GetProvider()

		// Check if it's a directory or file
		if isDirectory, _ := c.extractBoolParam(args, "is_directory", false, false); isDirectory {
			// Get all files from directory
			files, err := getFilesFromDirectory(filePath)
			if err != nil {
				return "", fmt.Errorf("failed to list files in directory %s: %w", filePath, err)
			}

			fileIDs, err := provider.IngestFiles(ctx, files, nil)
			if err != nil {
				return "", fmt.Errorf("failed to ingest files from directory %s: %w", filePath, err)
			}
			return fmt.Sprintf("Successfully ingested %d files from directory: %s", len(fileIDs), filePath), nil
		} else {
			fileID, err := provider.IngestFile(ctx, filePath, nil)
			if err != nil {
				return "", fmt.Errorf("failed to ingest file %s: %w", filePath, err)
			}
			return fmt.Sprintf("Successfully ingested file: %s (ID: %s)", filePath, fileID), nil
		}
	}

	// Fall back to RAGProvider interface methods
	// Check if it's a directory or file
	if isDirectory, _ := c.extractBoolParam(args, "is_directory", false, false); isDirectory {
		count, err := c.provider.IngestDirectory(ctx, filePath)
		if err != nil {
			return "", fmt.Errorf("failed to ingest directory %s: %w", filePath, err)
		}
		return fmt.Sprintf("Successfully ingested %d files from directory: %s", count, filePath), nil
	} else {
		err := c.provider.IngestPDF(ctx, filePath)
		if err != nil {
			return "", fmt.Errorf("failed to ingest file %s: %w", filePath, err)
		}
		return fmt.Sprintf("Successfully ingested file: %s", filePath), nil
	}
}

// handleRAGStats returns statistics about the knowledge base
func (c *Client) handleRAGStats(ctx context.Context, args map[string]interface{}) (string, error) {
	// Check if provider is a VectorProviderAdapter
	if adapter, ok := c.provider.(*VectorProviderAdapter); ok {
		provider := adapter.GetProvider()
		stats, err := provider.GetStats(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to get stats: %w", err)
		}

		var result strings.Builder
		result.WriteString("Vector Store Statistics:\n")
		if stats.TotalFiles >= 0 {
			result.WriteString(fmt.Sprintf("  Total Files: %d\n", stats.TotalFiles))
		}
		result.WriteString(fmt.Sprintf("  Total Chunks: %d\n", stats.TotalChunks))
		if stats.ProcessingFiles > 0 {
			result.WriteString(fmt.Sprintf("  Processing: %d\n", stats.ProcessingFiles))
		}
		if stats.FailedFiles > 0 {
			result.WriteString(fmt.Sprintf("  Failed: %d\n", stats.FailedFiles))
		}
		return result.String(), nil
	}

	// Fall back to RAGProvider interface
	stats, err := c.provider.GetStats(ctx)
	if err != nil {
		// Try just getting document count
		count, err := c.provider.GetDocumentCount(ctx)
		if err != nil {
			return "Statistics not available for current provider", nil
		}
		return fmt.Sprintf("Knowledge base contains %d documents", count), nil
	}

	// Format full stats
	var result strings.Builder
	result.WriteString("Knowledge Base Statistics:\n")
	result.WriteString(fmt.Sprintf("  Documents: %d\n", stats.DocumentCount))
	if stats.DatabaseSize > 0 {
		result.WriteString(fmt.Sprintf("  Database Size: %d bytes\n", stats.DatabaseSize))
	}
	if len(stats.FileTypeCounts) > 0 {
		result.WriteString("  File Types:\n")
		for fileType, count := range stats.FileTypeCounts {
			result.WriteString(fmt.Sprintf("    %s: %d\n", fileType, count))
		}
	}
	return result.String(), nil
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
// Returns nil if not using SimpleRAG
func (c *Client) GetRAG() *SimpleRAG {
	// Check if it's directly SimpleRAG
	if simpleRAG, ok := c.provider.(*SimpleRAG); ok {
		return simpleRAG
	}

	// Check if it's wrapped in LangChainRAGAdapter
	if adapter, ok := c.provider.(*LangChainRAGAdapter); ok {
		return adapter.simpleRAG
	}

	return nil
}

// GetProvider returns the underlying RAG provider
func (c *Client) GetProvider() RAGProvider {
	return c.provider
}

// formatVectorSearchResults formats vector search results for display
func (c *Client) formatVectorSearchResults(results []SearchResult, query string) string {
	if len(results) == 0 {
		return fmt.Sprintf("No relevant context found for query: '%s'", query)
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Found %d relevant context(s) for '%s':\n\n", len(results), query))

	for i, res := range results {
		result.WriteString(fmt.Sprintf("--- Context %d ---\n", i+1))

		// Add source information if available
		if res.FileName != "" {
			result.WriteString(fmt.Sprintf("Source: %s", res.FileName))
			if res.Score > 0 {
				result.WriteString(fmt.Sprintf(" (score: %.2f)", res.Score))
			}
			result.WriteString("\n")
		}

		// Add content with reasonable truncation
		content := strings.TrimSpace(res.Content)
		if len(content) > 800 {
			content = content[:800] + "..."
		}
		result.WriteString(fmt.Sprintf("Content: %s\n", content))

		// Add highlights if available
		if len(res.Highlights) > 0 {
			result.WriteString(fmt.Sprintf("Highlights: %s\n", strings.Join(res.Highlights, " | ")))
		}

		result.WriteString("\n")
	}

	return result.String()
}

// SetMaxDocs configures the maximum number of documents to return in search results
func (c *Client) SetMaxDocs(max int) {
	if max > 0 {
		c.maxDocs = max
	}
}
