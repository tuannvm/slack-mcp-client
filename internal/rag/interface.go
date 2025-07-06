// Package rag provides interfaces and types for RAG implementations
package rag

import (
	"context"
	"fmt"
	"time"

	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/vectorstores"
)

// RAGProvider defines a LangChain Go compatible vector store interface
// This aligns with github.com/tmc/langchaingo/vectorstores.VectorStore
type RAGProvider interface {
	// Core LangChain Go VectorStore interface
	AddDocuments(ctx context.Context, docs []schema.Document, options ...vectorstores.Option) ([]string, error)
	SimilaritySearch(ctx context.Context, query string, numDocuments int, options ...vectorstores.Option) ([]schema.Document, error)

	// Extended interface for RAG management
	RAGManager
}

// RAGManager provides additional RAG-specific operations
type RAGManager interface {
	// Document management
	IngestPDF(ctx context.Context, filePath string, options ...IngestOption) error
	IngestDirectory(ctx context.Context, dirPath string, options ...IngestOption) (int, error)
	DeleteDocuments(ctx context.Context, ids []string) error

	// Statistics and monitoring
	GetDocumentCount(ctx context.Context) (int, error)
	GetStats(ctx context.Context) (*RAGStats, error)

	// Lifecycle management
	Close() error
}

// IngestOption configures document ingestion behavior
type IngestOption func(*IngestConfig)

// IngestConfig holds ingestion configuration
type IngestConfig struct {
	ChunkSize     int
	ChunkOverlap  int
	Metadata      map[string]string
	ProcessorType string
}

// WithChunkSize sets the text chunking size
func WithChunkSize(size int) IngestOption {
	return func(c *IngestConfig) {
		c.ChunkSize = size
	}
}

// WithChunkOverlap sets the chunk overlap size
func WithChunkOverlap(overlap int) IngestOption {
	return func(c *IngestConfig) {
		c.ChunkOverlap = overlap
	}
}

// WithMetadata adds metadata to ingested documents
func WithMetadata(metadata map[string]string) IngestOption {
	return func(c *IngestConfig) {
		c.Metadata = metadata
	}
}

// RAGStats provides statistics about the RAG system
type RAGStats struct {
	DocumentCount    int64          `json:"document_count"`
	DatabaseSize     int64          `json:"database_size_bytes,omitempty"`
	IndexSize        int64          `json:"index_size_bytes,omitempty"`
	LastIngestion    *time.Time     `json:"last_ingestion,omitempty"`
	FileTypeCounts   map[string]int `json:"file_type_counts,omitempty"`
	AverageQueryTime time.Duration  `json:"avg_query_time,omitempty"`
	PopularQueries   []QueryStat    `json:"popular_queries,omitempty"`
}

// QueryStat tracks query usage statistics
type QueryStat struct {
	Query    string        `json:"query"`
	Count    int           `json:"count"`
	AvgTime  time.Duration `json:"avg_time"`
	LastUsed time.Time     `json:"last_used"`
}

// ProviderType represents different RAG backend types
type ProviderType string

const (
	ProviderTypeJSON     ProviderType = "json"
	ProviderTypeSQLite   ProviderType = "sqlite"
	ProviderTypeRedis    ProviderType = "redis"
	ProviderTypeElastic  ProviderType = "elastic"
	ProviderTypeChroma   ProviderType = "chroma"
	ProviderTypePinecone ProviderType = "pinecone"
	ProviderTypeSimple   ProviderType = "simple"
)

// ProviderConfig holds provider-specific configuration
type ProviderConfig struct {
	Provider     string                 `json:"provider"`     // Provider name: "simple", "openai", etc.
	DatabasePath string                 `json:"database_path"`
	Options      map[string]interface{} `json:"options"`
}


// NewLangChainCompatibleRAG creates a RAG provider that's compatible with LangChain Go
func NewLangChainCompatibleRAG(config ProviderConfig) (RAGProvider, error) {
	return &LangChainRAGAdapter{
		simpleRAG: NewSimpleRAG(config.DatabasePath),
		config:    config,
	}, nil
}

// LangChainRAGAdapter adapts SimpleRAG to be LangChain Go compatible
type LangChainRAGAdapter struct {
	simpleRAG *SimpleRAG
	config    ProviderConfig
}

// AddDocuments implements the LangChain Go VectorStore interface
func (l *LangChainRAGAdapter) AddDocuments(ctx context.Context, docs []schema.Document, options ...vectorstores.Option) ([]string, error) {
	// Convert schema.Document to our Document format
	ids := make([]string, len(docs))

	for i, doc := range docs {
		// Convert metadata from map[string]any to map[string]string
		metadata := make(map[string]string)
		for k, v := range doc.Metadata {
			if str, ok := v.(string); ok {
				metadata[k] = str
			} else {
				metadata[k] = fmt.Sprintf("%v", v)
			}
		}

		// Add document to our internal storage
		ourDoc := Document{
			Content:  doc.PageContent,
			Metadata: metadata,
		}

		l.simpleRAG.documents = append(l.simpleRAG.documents, ourDoc)
		ids[i] = fmt.Sprintf("doc_%d", len(l.simpleRAG.documents))
	}

	// Save to persistent storage
	if err := l.simpleRAG.save(); err != nil {
		return nil, fmt.Errorf("failed to save documents: %w", err)
	}

	return ids, nil
}

// SimilaritySearch implements the LangChain Go VectorStore interface
func (l *LangChainRAGAdapter) SimilaritySearch(ctx context.Context, query string, numDocuments int, options ...vectorstores.Option) ([]schema.Document, error) {
	// Use our existing search functionality
	results := l.simpleRAG.Search(query, numDocuments)

	// Convert our Document format to schema.Document
	docs := make([]schema.Document, len(results))
	for i, result := range results {
		// Convert metadata from map[string]string to map[string]any
		metadata := make(map[string]any)
		for k, v := range result.Metadata {
			metadata[k] = v
		}

		docs[i] = schema.Document{
			PageContent: result.Content,
			Metadata:    metadata,
		}
	}

	return docs, nil
}

// IngestPDF implements RAGManager interface
func (l *LangChainRAGAdapter) IngestPDF(ctx context.Context, filePath string, options ...IngestOption) error {
	// Apply options
	config := &IngestConfig{
		ChunkSize:    1000,
		ChunkOverlap: 200,
	}
	for _, opt := range options {
		opt(config)
	}

	return l.simpleRAG.IngestPDF(ctx, filePath, options...)
}

// IngestDirectory implements RAGManager interface
func (l *LangChainRAGAdapter) IngestDirectory(ctx context.Context, dirPath string, options ...IngestOption) (int, error) {
	// Apply options
	config := &IngestConfig{
		ChunkSize:    1000,
		ChunkOverlap: 200,
	}
	for _, opt := range options {
		opt(config)
	}

	return l.simpleRAG.IngestDirectory(ctx, dirPath, options...)
}

// DeleteDocuments implements RAGManager interface
func (l *LangChainRAGAdapter) DeleteDocuments(ctx context.Context, ids []string) error {
	// For SimpleRAG, we don't track IDs, so this is a no-op for now
	// In a real implementation, you'd track document IDs and remove them
	return fmt.Errorf("delete documents not implemented for SimpleRAG")
}

// GetDocumentCount implements RAGManager interface
func (l *LangChainRAGAdapter) GetDocumentCount(ctx context.Context) (int, error) {
	return l.simpleRAG.GetDocumentCountSimple(), nil
}

// GetStats implements RAGManager interface
func (l *LangChainRAGAdapter) GetStats(ctx context.Context) (*RAGStats, error) {
	count := l.simpleRAG.GetDocumentCountSimple()
	return &RAGStats{
		DocumentCount: int64(count),
		FileTypeCounts: map[string]int{
			"pdf": count, // For now, assume all are PDFs
		},
	}, nil
}

// Close implements RAGManager interface
func (l *LangChainRAGAdapter) Close() error {
	// SimpleRAG doesn't need cleanup, but interface requires it
	return nil
}

// Convenience functions for creating LangChain Go compatible retrievers

// ToRetriever converts a RAGProvider to a LangChain Go Retriever
func ToRetriever(provider RAGProvider, numDocuments int, options ...vectorstores.Option) vectorstores.Retriever {
	return vectorstores.ToRetriever(provider, numDocuments, options...)
}
