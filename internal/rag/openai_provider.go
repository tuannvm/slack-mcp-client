// Package rag provides OpenAI vector store implementation
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

// OpenAIProvider implements VectorProvider using OpenAI's Assistants API with file search
type OpenAIProvider struct {
	client        openai.Client
	assistantID   string
	vectorStoreID string
	config        OpenAIConfig
}

// OpenAIConfig holds configuration for the OpenAI provider
type OpenAIConfig struct {
	APIKey          string
	AssistantID     string // Optional: reuse existing assistant
	VectorStoreID   string // Optional: reuse existing vector store
	AssistantName   string // Name for the assistant (default: "RAG Assistant")
	VectorStoreName string // Name for the vector store (default: "Knowledge Base")
	Model           string // Default: gpt-4-turbo
	MaxResults      int    // Default: 20
	CustomPrompt    string // Custom instruction prompt for the assistant
}

// NewOpenAIProvider creates a new OpenAI vector provider instance
func NewOpenAIProvider(config map[string]interface{}) (VectorProvider, error) {
	cfg := OpenAIConfig{
		Model:           "gpt-4-turbo",
		MaxResults:      20,
		AssistantName:   "RAG Assistant",
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

	if assistantID, ok := config["assistant_id"].(string); ok {
		cfg.AssistantID = assistantID
	}

	if vectorStoreID, ok := config["vector_store_id"].(string); ok {
		cfg.VectorStoreID = vectorStoreID
	}

	if assistantName, ok := config["assistant_name"].(string); ok {
		cfg.AssistantName = assistantName
	}

	if vectorStoreName, ok := config["vector_store_name"].(string); ok {
		cfg.VectorStoreName = vectorStoreName
	}

	if model, ok := config["model"].(string); ok {
		cfg.Model = model
	}

	if maxResults, ok := config["max_results"].(int); ok {
		cfg.MaxResults = maxResults
	}

	if customPrompt, ok := config["custom_prompt"].(string); ok {
		cfg.CustomPrompt = customPrompt
	} else if customPromptArray, ok := config["custom_prompt"].([]interface{}); ok {
		// Handle array format - join with newlines
		var promptLines []string
		for _, line := range customPromptArray {
			if str, ok := line.(string); ok {
				promptLines = append(promptLines, str)
			}
		}
		cfg.CustomPrompt = strings.Join(promptLines, "\n")
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

// Initialize sets up the OpenAI assistant and vector store
func (o *OpenAIProvider) Initialize(ctx context.Context) error {
	// Find or create assistant
	if o.config.AssistantID != "" {
		// Use specific assistant ID
		assistant, err := o.client.Beta.Assistants.Get(ctx, o.config.AssistantID)
		if err != nil {
			return fmt.Errorf("failed to retrieve assistant: %w", err)
		}
		o.assistantID = assistant.ID
		fmt.Printf("[RAG] OpenAI: Using existing assistant '%s' with ID: %s\n", assistant.Name, o.assistantID)
	} else {
		// Search for existing assistant by name first
		existingAssistant, err := o.findAssistantByName(ctx, o.config.AssistantName)
		if err != nil {
			return fmt.Errorf("failed to search for assistant: %w", err)
		}

		if existingAssistant != nil {
			// Found existing assistant
			o.assistantID = existingAssistant.ID
			fmt.Printf("[RAG] OpenAI: Found existing assistant '%s' with ID: %s\n", o.config.AssistantName, o.assistantID)
		} else {
			// Create new assistant
			instructions := "You are a helpful assistant that searches through uploaded documents to answer questions."
			if o.config.CustomPrompt != "" {
				instructions = o.config.CustomPrompt
			}

			assistant, err := o.client.Beta.Assistants.New(ctx, openai.BetaAssistantNewParams{
				Model:        o.config.Model,
				Name:         openai.String(o.config.AssistantName),
				Instructions: openai.String(instructions),
				Tools: []openai.AssistantToolUnionParam{
					{
						OfFileSearch: &openai.FileSearchToolParam{
							Type: "file_search",
						},
					},
				},
			})
			if err != nil {
				return fmt.Errorf("failed to create assistant: %w", err)
			}
			o.assistantID = assistant.ID
			fmt.Printf("[RAG] OpenAI: Created new assistant '%s' with ID: %s\n", o.config.AssistantName, o.assistantID)
		}
	}

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

	// Update assistant with vector store
	_, err := o.client.Beta.Assistants.Update(ctx, o.assistantID, openai.BetaAssistantUpdateParams{
		ToolResources: openai.BetaAssistantUpdateParamsToolResources{
			FileSearch: openai.BetaAssistantUpdateParamsToolResourcesFileSearch{
				VectorStoreIDs: []string{o.vectorStoreID},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to attach vector store to assistant: %w", err)
	}

	return nil
}

// IngestFile uploads a file to the OpenAI vector store
func (o *OpenAIProvider) IngestFile(ctx context.Context, filePath string, metadata map[string]string) (string, error) {
	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Printf("Warning: failed to close file: %v\n", err)
		}
	}()

	// Upload file with purpose "assistants"
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

// Search performs a vector search using OpenAI's assistant
func (o *OpenAIProvider) Search(ctx context.Context, query string, options SearchOptions) ([]SearchResult, error) {
	fmt.Printf("[RAG] OpenAI: Starting vector search (assistant: %s, vector_store: %s)\n",
		o.assistantID, o.vectorStoreID)

	// Create a new thread for the search
	thread, err := o.client.Beta.Threads.New(ctx, openai.BetaThreadNewParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to create thread: %w", err)
	}

	fmt.Printf("[RAG] OpenAI: Created thread %s for search\n", thread.ID)

	// Add user's search query
	_, err = o.client.Beta.Threads.Messages.New(ctx, thread.ID, openai.BetaThreadMessageNewParams{
		Role: openai.BetaThreadMessageNewParamsRoleUser,
		Content: openai.BetaThreadMessageNewParamsContentUnion{
			OfString: openai.String(query),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create message: %w", err)
	}

	// Run assistant with file_search tool to find relevant documents
	searchInstructions := "Search through all uploaded documents to find information relevant to the user's query. Return specific excerpts, data points, and context from the documents. Include document names and page references when possible. Provide comprehensive information even if it spans multiple documents."

	// Use custom prompt if available for more targeted search
	if o.config.CustomPrompt != "" {
		searchInstructions = o.config.CustomPrompt + "\n\nFor this specific search, focus on finding the exact information requested in the user's query. Extract relevant data, metrics, and context from the documents."
	}

	run, err := o.client.Beta.Threads.Runs.New(ctx, thread.ID, openai.BetaThreadRunNewParams{
		AssistantID:  o.assistantID,
		Instructions: openai.String(searchInstructions),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create run: %w", err)
	}

	// Poll for completion
	for {
		run, err = o.client.Beta.Threads.Runs.Get(ctx, thread.ID, run.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get run status: %w", err)
		}

		if run.Status == openai.RunStatusCompleted {
			break
		} else if run.Status == openai.RunStatusFailed {
			return nil, fmt.Errorf("run failed: %v", run.LastError)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(1 * time.Second):
			// Continue polling
		}
	}

	// Extract file chunks/citations from the response
	messages, err := o.client.Beta.Threads.Messages.List(ctx, thread.ID, openai.BetaThreadMessageListParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to list messages: %w", err)
	}

	// Return whatever OpenAI gives us directly - no manipulation
	results := make([]SearchResult, 0)

	for _, msg := range messages.Data {
		// Skip user messages, we only want assistant responses
		if msg.Role == "user" {
			continue
		}

		// Get the raw content from OpenAI and return it as-is
		for _, content := range msg.Content {
			if content.Type == "text" {
				result := SearchResult{
					Content:  content.Text.Value,
					Score:    0.8,
					Metadata: make(map[string]string),
				}
				results = append(results, result)
			}
		}
	}

	fmt.Printf("[RAG] OpenAI: Search completed. Found %d results from vector store\n", len(results))
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

// GetAssistantID returns the OpenAI assistant ID
func (o *OpenAIProvider) GetAssistantID() string {
	return o.assistantID
}

// GetVectorStoreID returns the OpenAI vector store ID
func (o *OpenAIProvider) GetVectorStoreID() string {
	return o.vectorStoreID
}

// findAssistantByName searches for an existing assistant by name
func (o *OpenAIProvider) findAssistantByName(ctx context.Context, name string) (*openai.Assistant, error) {
	// List assistants and search for matching name
	assistants, err := o.client.Beta.Assistants.List(ctx, openai.BetaAssistantListParams{
		Limit: openai.Int(100), // Get up to 100 assistants to search through
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list assistants: %w", err)
	}

	// Search through assistants for matching name
	for _, assistant := range assistants.Data {
		if assistant.Name == name {
			return &assistant, nil
		}
	}

	return nil, nil // Not found
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
