// Package rag provides OpenAI vector store implementation with 2025 API updates
package rag

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// OpenAIConfig holds configuration for the OpenAI provider
type OpenAIConfig struct {
	APIKey          string
	VectorStoreID   string  // Optional: reuse existing vector store
	VectorStoreName string  // Name for the vector store (default: "Knowledge Base")
	MaxResults      int64   // Default: 20
	ScoreThreshold  float64 // Default: 0.5
}

// OpenAIProvider implements VectorProvider using OpenAI's VectorStore API with 2025 updates
type OpenAIProvider struct {
	client        openai.Client
	vectorStoreID string
	config        OpenAIConfig
}

// NewOpenAIProvider creates a new OpenAI vector provider instance
func NewOpenAIProvider(config map[string]interface{}) (VectorProvider, error) {
	cfg := OpenAIConfig{
		MaxResults:      20,
		ScoreThreshold:  0.5,
		VectorStoreName: "Knowledge Base",
	}

	// Extract configuration
	if apiKey, ok := config["api_key"].(string); ok {
		cfg.APIKey = apiKey
	} else {
		// Try environment variable
		cfg.APIKey = os.Getenv("OPENAI_API_KEY")
	}

	if cfg.APIKey == "" {
		return nil, fmt.Errorf("OpenAI API key not provided")
	}

	if vectorStoreID, ok := config["vector_store_id"].(string); ok {
		cfg.VectorStoreID = vectorStoreID
	}

	if vectorStoreName, ok := config["vector_store_name"].(string); ok {
		cfg.VectorStoreName = vectorStoreName
	}

	if scoreThreshold, ok := config["score_threshold"].(float64); ok {
		cfg.ScoreThreshold = scoreThreshold
	}

	if maxResults, ok := config["max_results"].(float64); ok {
		cfg.MaxResults = int64(maxResults)
	}

	// Create OpenAI client
	client := openai.NewClient(
		option.WithAPIKey(cfg.APIKey),
	)

	return &OpenAIProvider{
		client: client,
		config: cfg,
	}, nil
}

// Initialize sets up the OpenAI vector store only
func (o *OpenAIProvider) Initialize(ctx context.Context) error {
	// Find or create vector store
	if o.config.VectorStoreID != "" {
		// Use specific vector store ID
		vectorStore, err := o.client.VectorStores.Get(ctx, o.config.VectorStoreID)
		if err != nil {
			return fmt.Errorf("failed to retrieve vector store: %w", err)
		}
		o.vectorStoreID = vectorStore.ID
		fmt.Printf("[RAG] OpenAI: Using existing vector store '%s' with ID: %s\n", vectorStore.Name, o.vectorStoreID)
	} else {
		// Search for existing vector store by name first
		existingVectorStore, err := o.findVectorStoreByName(ctx, o.config.VectorStoreName)
		if err != nil {
			return fmt.Errorf("failed to search for vector store: %w", err)
		}

		if existingVectorStore != nil {
			// Found existing vector store
			o.vectorStoreID = existingVectorStore.ID
			fmt.Printf("[RAG] OpenAI: Found existing vector store '%s' with ID: %s\n", o.config.VectorStoreName, o.vectorStoreID)
		} else {
			// Create new vector store
			vectorStore, err := o.client.VectorStores.New(ctx, openai.VectorStoreNewParams{
				Name: openai.String(o.config.VectorStoreName),
			})
			if err != nil {
				return fmt.Errorf("failed to create vector store: %w", err)
			}
			o.vectorStoreID = vectorStore.ID
			fmt.Printf("[RAG] OpenAI: Created new vector store '%s' with ID: %s\n", o.config.VectorStoreName, o.vectorStoreID)
		}
	}

	return nil
}

// IngestFile uploads a file to the OpenAI vector store
func (o *OpenAIProvider) IngestFile(ctx context.Context, filePath string, metadata map[string]string) (string, error) {
	// Open the file for upload
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Printf("Warning: failed to close file: %v\n", err)
		}
	}()

	// Upload file with purpose "assistants" for vector store use
	uploadedFile, err := o.client.Files.New(ctx, openai.FileNewParams{
		File:    file,
		Purpose: openai.FilePurposeAssistants,
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload file: %w", err)
	}

	// Attach file to vector store
	vectorStoreFile, err := o.client.VectorStores.Files.New(ctx, o.vectorStoreID, openai.VectorStoreFileNewParams{
		FileID: uploadedFile.ID,
	})
	if err != nil {
		return "", fmt.Errorf("failed to attach file to vector store: %w", err)
	}

	// Poll for completion
	for {
		vsFile, err := o.client.VectorStores.Files.Get(ctx, o.vectorStoreID, vectorStoreFile.ID)
		if err != nil {
			return "", fmt.Errorf("failed to check file status: %w", err)
		}

		if string(vsFile.Status) == "completed" {
			break
		} else if string(vsFile.Status) == "failed" {
			return "", fmt.Errorf("file processing failed")
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(2 * time.Second):
			// Continue polling
		}
	}

	return uploadedFile.ID, nil
}

// IngestFiles uploads multiple files to the OpenAI vector store
func (o *OpenAIProvider) IngestFiles(ctx context.Context, filePaths []string, metadata map[string]string) ([]string, error) {
	fileIDs := make([]string, 0, len(filePaths))

	for _, filePath := range filePaths {
		fileID, err := o.IngestFile(ctx, filePath, metadata)
		if err != nil {
			// Log error but continue with other files
			fmt.Printf("Warning: failed to ingest %s: %v\n", filePath, err)
			continue
		}
		fileIDs = append(fileIDs, fileID)
	}

	return fileIDs, nil
}

// DeleteFile removes a file from the vector store
func (o *OpenAIProvider) DeleteFile(ctx context.Context, fileID string) error {
	// Remove from vector store first
	_, err := o.client.VectorStores.Files.Delete(ctx, o.vectorStoreID, fileID)
	if err != nil {
		return fmt.Errorf("failed to remove file from vector store: %w", err)
	}

	// Delete the file itself
	_, err = o.client.Files.Delete(ctx, fileID)
	if err != nil {
		// Log but don't fail if file deletion fails
		fmt.Printf("Warning: failed to delete file %s: %v\n", fileID, err)
	}

	return nil
}

// ListFiles lists all files in the vector store
func (o *OpenAIProvider) ListFiles(ctx context.Context, limit int) ([]FileInfo, error) {
	// List vector store files
	vsFiles, err := o.client.VectorStores.Files.List(ctx, o.vectorStoreID, openai.VectorStoreFileListParams{
		Limit: openai.Int(int64(limit)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list vector store files: %w", err)
	}

	files := make([]FileInfo, 0)
	for _, vsFile := range vsFiles.Data {
		// Get file details
		file, err := o.client.Files.Get(ctx, vsFile.ID)
		if err != nil {
			// Skip files we can't get details for
			continue
		}

		files = append(files, FileInfo{
			ID:         vsFile.ID,
			Name:       file.Filename,
			Size:       int64(file.Bytes),
			UploadedAt: time.Unix(file.CreatedAt, 0),
			Status:     string(vsFile.Status),
			Metadata:   make(map[string]string), // OpenAI doesn't support custom metadata on files
		})
	}

	return files, nil
}

// Search performs semantic search using OpenAI's Vector Store Search API (2025)
func (o *OpenAIProvider) Search(ctx context.Context, query string, options SearchOptions) ([]SearchResult, error) {
	fmt.Printf("[RAG] OpenAI: Vector Store search for query '%s' (vector_store: %s)\n", query, o.vectorStoreID)

	// Set up search parameters
	limit := o.config.MaxResults
	if o.config.MaxResults < 0 {
		limit = 20
	}

	scoreThreshold := o.config.ScoreThreshold
	if scoreThreshold < 0 {
		scoreThreshold = 0.1
	}

	// Use OpenAI's Vector Store Search API with proper union type
	searchParams := openai.VectorStoreSearchParams{
		Query: openai.VectorStoreSearchParamsQueryUnion{
			OfString: openai.String(query),
		},
		MaxNumResults: openai.Int(limit),
		RankingOptions: openai.VectorStoreSearchParamsRankingOptions{
			ScoreThreshold: openai.Float(scoreThreshold),
			Ranker:         "auto",
		},
	}

	searchResults, err := o.client.VectorStores.Search(ctx, o.vectorStoreID, searchParams)
	if err != nil {
		return nil, fmt.Errorf("vector store search failed: %w", err)
	}

	// Process search results
	results := make([]SearchResult, 0)

	for i, result := range searchResults.Data {
		// Extract content from the response
		var content string
		if len(result.Content) > 0 {
			// Combine all content pieces
			var contentParts []string
			for _, contentItem := range result.Content {
				if contentItem.Text != "" {
					contentParts = append(contentParts, contentItem.Text)
				}
			}
			content = strings.Join(contentParts, "\n")
		} else {
			content = "No content available"
		}

		searchResult := SearchResult{
			Content:  content,
			Score:    float32(result.Score),
			FileName: result.Filename,
			Metadata: map[string]string{
				"vector_store_id": o.vectorStoreID,
				"query":           query,
				"result_index":    fmt.Sprintf("%d", i),
				"score":           fmt.Sprintf("%.4f", result.Score),
			},
			Highlights: strings.Fields(strings.ToLower(query)),
		}

		// Add file metadata if available
		if result.FileID != "" {
			searchResult.Metadata["file_id"] = result.FileID
		}
		if result.Filename != "" {
			searchResult.Metadata["file_name"] = result.Filename
		}

		results = append(results, searchResult)
	}

	fmt.Printf("[RAG] OpenAI: Vector Store search completed. Found %d results\n", len(results))
	return results, nil
}

// GetStats returns statistics about the vector store
func (o *OpenAIProvider) GetStats(ctx context.Context) (*VectorStoreStats, error) {
	// Get vector store details
	vs, err := o.client.VectorStores.Get(ctx, o.vectorStoreID)
	if err != nil {
		return nil, fmt.Errorf("failed to get vector store: %w", err)
	}

	stats := &VectorStoreStats{
		LastUpdated: time.Now(),
	}

	// Extract file counts from the response
	stats.TotalFiles = int(vs.FileCounts.Total)
	stats.ProcessingFiles = int(vs.FileCounts.InProgress)
	stats.FailedFiles = int(vs.FileCounts.Failed)
	stats.TotalChunks = int(vs.FileCounts.Completed) // Approximate chunks by completed files

	return stats, nil
}

// Close cleans up resources (no-op for OpenAI)
func (o *OpenAIProvider) Close() error {
	// OpenAI client doesn't need explicit cleanup
	return nil
}

// GetVectorStoreID returns the OpenAI vector store ID
func (o *OpenAIProvider) GetVectorStoreID() string {
	return o.vectorStoreID
}

// findVectorStoreByName searches for an existing vector store by name
func (o *OpenAIProvider) findVectorStoreByName(ctx context.Context, name string) (*openai.VectorStore, error) {
	// List vector stores and search for matching name
	vectorStores, err := o.client.VectorStores.List(ctx, openai.VectorStoreListParams{
		Limit: openai.Int(100), // Get up to 100 vector stores to search through
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list vector stores: %w", err)
	}

	// Search through vector stores for matching name
	for _, vs := range vectorStores.Data {
		if vs.Name == name {
			return &vs, nil
		}
	}

	return nil, nil // Not found
}

// init registers the OpenAI provider
func init() {
	RegisterVectorProvider("openai", NewOpenAIProvider)
}
