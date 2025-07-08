// Package rag provides interfaces and implementations for Retrieval-Augmented Generation
package rag

import (
	"context"
	"time"
)

// VectorProvider defines the interface that all vector store providers must implement
// This abstraction allows switching between different vector stores (OpenAI, ChromaDB, etc.)
type VectorProvider interface {
	// Initialize sets up the vector provider (e.g., create assistant, vector store)
	Initialize(ctx context.Context) error

	// Document Management
	IngestFile(ctx context.Context, filePath string, metadata map[string]string) (string, error)
	IngestFiles(ctx context.Context, filePaths []string, metadata map[string]string) ([]string, error)
	DeleteFile(ctx context.Context, fileID string) error
	ListFiles(ctx context.Context, limit int) ([]FileInfo, error)

	// Search Operations
	Search(ctx context.Context, query string, options SearchOptions) ([]SearchResult, error)

	// Lifecycle
	Close() error

	// Metadata
	GetStats(ctx context.Context) (*VectorStoreStats, error)
}

// FileInfo represents information about a file in the vector store
type FileInfo struct {
	ID         string
	Name       string
	Size       int64
	UploadedAt time.Time
	Metadata   map[string]string
	Status     string // processing, completed, failed
}

// SearchOptions configures search parameters
type SearchOptions struct {
	Limit    int               // Maximum number of results
	MinScore float32           // Minimum relevance score
	Metadata map[string]string // Filter by metadata
}

// SearchResult represents a search result from the vector store
type SearchResult struct {
	Content    string            // The actual content
	Score      float32           // Relevance score
	FileID     string            // ID of the source file
	FileName   string            // Name of the source file
	Metadata   map[string]string // Additional metadata
	Highlights []string          // Highlighted snippets
}

// VectorStoreStats provides statistics about the vector store
type VectorStoreStats struct {
	TotalFiles       int
	TotalChunks      int
	ProcessingFiles  int
	FailedFiles      int
	StorageSizeBytes int64
	LastUpdated      time.Time
}

// VectorProviderFactory creates vector provider instances
type VectorProviderFactory func(config map[string]interface{}) (VectorProvider, error)

// providerRegistry stores registered vector provider factories
var providerRegistry = make(map[string]VectorProviderFactory)

// RegisterVectorProvider registers a new vector provider factory
func RegisterVectorProvider(name string, factory VectorProviderFactory) {
	providerRegistry[name] = factory
}

// Note: CreateVectorProvider is now in factory.go

// ProviderNotFoundError indicates a requested provider is not registered
type ProviderNotFoundError struct {
	Provider string
}

func (e *ProviderNotFoundError) Error() string {
	return "vector provider not found: " + e.Provider
}
