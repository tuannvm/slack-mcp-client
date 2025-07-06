// Package rag provides adapters to bridge new vector providers with existing interfaces
package rag

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/vectorstores"
)

// VectorProviderAdapter adapts VectorProvider to the existing RAGProvider interface
// This ensures backward compatibility with existing code using RAGProvider
type VectorProviderAdapter struct {
	provider    VectorProvider
	tempDir     string
	fileMapping map[string]string // Maps document IDs to file IDs
}

// NewVectorProviderAdapter creates a new adapter for a vector provider
func NewVectorProviderAdapter(provider VectorProvider) (*VectorProviderAdapter, error) {
	// Create temp directory for document-to-file conversions
	tempDir, err := os.MkdirTemp("", "rag-adapter-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	return &VectorProviderAdapter{
		provider:    provider,
		tempDir:     tempDir,
		fileMapping: make(map[string]string),
	}, nil
}

// AddDocuments implements RAGProvider interface by converting documents to files
func (a *VectorProviderAdapter) AddDocuments(ctx context.Context, docs []schema.Document, options ...vectorstores.Option) ([]string, error) {
	// Create temporary files for each document
	tempFiles := make([]string, 0, len(docs))
	docIDs := make([]string, 0, len(docs))

	for i, doc := range docs {
		// Generate unique filename
		filename := fmt.Sprintf("doc_%d.txt", i)
		if title, ok := doc.Metadata["title"].(string); ok && title != "" {
			// Sanitize title for filename
			filename = strings.ReplaceAll(title, "/", "_")
			filename = strings.ReplaceAll(filename, " ", "_")
			if !strings.HasSuffix(filename, ".txt") {
				filename += ".txt"
			}
		}

		// Create temp file
		tempPath := filepath.Join(a.tempDir, filename)
		if err := os.WriteFile(tempPath, []byte(doc.PageContent), 0644); err != nil {
			// Clean up previously created files
			a.cleanupTempFiles(tempFiles)
			return nil, fmt.Errorf("failed to create temp file: %w", err)
		}
		tempFiles = append(tempFiles, tempPath)
	}

	// Convert metadata
	metadata := a.extractCommonMetadata(docs)

	// Ingest files using the provider
	fileIDs, err := a.provider.IngestFiles(ctx, tempFiles, metadata)
	if err != nil {
		a.cleanupTempFiles(tempFiles)
		return nil, fmt.Errorf("failed to ingest files: %w", err)
	}

	// Generate document IDs and maintain mapping
	for i, fileID := range fileIDs {
		docID := fmt.Sprintf("doc_%s_%d", fileID, i)
		docIDs = append(docIDs, docID)
		a.fileMapping[docID] = fileID
	}

	// Clean up temp files
	a.cleanupTempFiles(tempFiles)

	return docIDs, nil
}

// SimilaritySearch implements RAGProvider interface using vector search
func (a *VectorProviderAdapter) SimilaritySearch(ctx context.Context, query string, numDocuments int, options ...vectorstores.Option) ([]schema.Document, error) {
	// Perform search using the provider
	results, err := a.provider.Search(ctx, query, SearchOptions{
		Limit: numDocuments,
	})
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	// Convert results to schema.Document format
	docs := make([]schema.Document, 0, len(results))
	for _, result := range results {
		metadata := make(map[string]interface{})

		// Copy metadata
		for k, v := range result.Metadata {
			metadata[k] = v
		}

		// Add additional fields
		metadata["file_id"] = result.FileID
		metadata["file_name"] = result.FileName
		metadata["score"] = result.Score

		// Add highlights if available
		if len(result.Highlights) > 0 {
			metadata["highlights"] = strings.Join(result.Highlights, " | ")
		}

		docs = append(docs, schema.Document{
			PageContent: result.Content,
			Metadata:    metadata,
		})
	}

	return docs, nil
}

// extractCommonMetadata extracts common metadata from documents
func (a *VectorProviderAdapter) extractCommonMetadata(docs []schema.Document) map[string]string {
	metadata := make(map[string]string)

	// Extract common metadata fields
	if len(docs) > 0 {
		firstDoc := docs[0]
		if source, ok := firstDoc.Metadata["source"].(string); ok {
			metadata["source"] = source
		}
		if docType, ok := firstDoc.Metadata["type"].(string); ok {
			metadata["type"] = docType
		}
	}

	return metadata
}

// cleanupTempFiles removes temporary files
func (a *VectorProviderAdapter) cleanupTempFiles(files []string) {
	for _, file := range files {
		os.Remove(file)
	}
}

// Close cleans up resources
func (a *VectorProviderAdapter) Close() error {
	// Clean up temp directory
	if a.tempDir != "" {
		os.RemoveAll(a.tempDir)
	}

	// Close the underlying provider
	if a.provider != nil {
		return a.provider.Close()
	}

	return nil
}

// GetProvider returns the underlying vector provider
func (a *VectorProviderAdapter) GetProvider() VectorProvider {
	return a.provider
}

// DeleteDocuments implements RAGProvider interface
func (a *VectorProviderAdapter) DeleteDocuments(ctx context.Context, ids []string) error {
	// Delete files by ID
	for _, docID := range ids {
		// Extract file ID from document ID mapping
		if fileID, exists := a.fileMapping[docID]; exists {
			if err := a.provider.DeleteFile(ctx, fileID); err != nil {
				return fmt.Errorf("failed to delete file %s: %w", fileID, err)
			}
			delete(a.fileMapping, docID)
		}
	}
	return nil
}

// Additional RAGManager methods for VectorProviderAdapter

// IngestPDF implements RAGManager interface
func (a *VectorProviderAdapter) IngestPDF(ctx context.Context, filePath string, options ...IngestOption) error {
	_, err := a.provider.IngestFile(ctx, filePath, nil)
	return err
}

// IngestDirectory implements RAGManager interface
func (a *VectorProviderAdapter) IngestDirectory(ctx context.Context, dirPath string, options ...IngestOption) (int, error) {
	files, err := getFilesFromDirectory(dirPath)
	if err != nil {
		return 0, err
	}

	fileIDs, err := a.provider.IngestFiles(ctx, files, nil)
	return len(fileIDs), err
}

// GetDocumentCount implements RAGManager interface
func (a *VectorProviderAdapter) GetDocumentCount(ctx context.Context) (int, error) {
	stats, err := a.provider.GetStats(ctx)
	if err != nil {
		return 0, err
	}
	return stats.TotalChunks, nil
}

// GetStats implements RAGManager interface
func (a *VectorProviderAdapter) GetStats(ctx context.Context) (*RAGStats, error) {
	stats, err := a.provider.GetStats(ctx)
	if err != nil {
		return nil, err
	}

	return &RAGStats{
		DocumentCount: int64(stats.TotalChunks),
		DatabaseSize:  stats.StorageSizeBytes,
		LastIngestion: &stats.LastUpdated,
	}, nil
}

// SimpleRAGAdapter adapts SimpleRAG to the VectorProvider interface
// This allows using SimpleRAG through the same interface as other providers
type SimpleRAGAdapter struct {
	rag *SimpleRAG
}

// NewSimpleRAGAdapter creates an adapter for SimpleRAG
func NewSimpleRAGAdapter(dbPath string) *SimpleRAGAdapter {
	return &SimpleRAGAdapter{
		rag: NewSimpleRAG(dbPath),
	}
}

// Initialize is a no-op for SimpleRAG
func (s *SimpleRAGAdapter) Initialize(ctx context.Context) error {
	// SimpleRAG doesn't need initialization
	return nil
}

// IngestFile processes a file using SimpleRAG
func (s *SimpleRAGAdapter) IngestFile(ctx context.Context, filePath string, metadata map[string]string) (string, error) {
	// SimpleRAG only supports PDF files
	if !strings.HasSuffix(strings.ToLower(filePath), ".pdf") {
		return "", fmt.Errorf("SimpleRAG only supports PDF files")
	}

	err := s.rag.IngestPDFSimple(filePath)
	if err != nil {
		return "", err
	}

	// Return the file path as ID
	return filePath, nil
}

// IngestFiles processes multiple files
func (s *SimpleRAGAdapter) IngestFiles(ctx context.Context, filePaths []string, metadata map[string]string) ([]string, error) {
	fileIDs := make([]string, 0, len(filePaths))

	for _, filePath := range filePaths {
		fileID, err := s.IngestFile(ctx, filePath, metadata)
		if err != nil {
			// Log error but continue
			fmt.Printf("Warning: failed to ingest %s: %v\n", filePath, err)
			continue
		}
		fileIDs = append(fileIDs, fileID)
	}

	return fileIDs, nil
}

// DeleteFile is not supported by SimpleRAG
func (s *SimpleRAGAdapter) DeleteFile(ctx context.Context, fileID string) error {
	return fmt.Errorf("delete not supported by SimpleRAG")
}

// ListFiles is not supported by SimpleRAG
func (s *SimpleRAGAdapter) ListFiles(ctx context.Context, limit int) ([]FileInfo, error) {
	return nil, fmt.Errorf("list files not supported by SimpleRAG")
}

// Search performs text-based search using SimpleRAG
func (s *SimpleRAGAdapter) Search(ctx context.Context, query string, options SearchOptions) ([]SearchResult, error) {
	// Use SimpleRAG's search
	docs := s.rag.Search(query, options.Limit)

	// Convert to SearchResult format
	results := make([]SearchResult, 0, len(docs))
	for _, doc := range docs {
		result := SearchResult{
			Content:  doc.Content,
			Score:    1.0, // SimpleRAG doesn't provide scores
			Metadata: doc.Metadata,
		}

		// Extract file info from metadata
		if fileName, ok := doc.Metadata["file_name"]; ok {
			result.FileName = fileName
		}
		if fileID, ok := doc.Metadata["file_path"]; ok {
			result.FileID = fileID
		}

		results = append(results, result)
	}

	return results, nil
}

// GetStats returns basic statistics for SimpleRAG
func (s *SimpleRAGAdapter) GetStats(ctx context.Context) (*VectorStoreStats, error) {
	count := s.rag.GetDocumentCountSimple()

	return &VectorStoreStats{
		TotalFiles:  -1, // Unknown for SimpleRAG
		TotalChunks: count,
		LastUpdated: time.Now(),
	}, nil
}

// Close is a no-op for SimpleRAG
func (s *SimpleRAGAdapter) Close() error {
	// SimpleRAG doesn't need cleanup
	return nil
}

// GetSimpleRAG returns the underlying SimpleRAG instance
func (s *SimpleRAGAdapter) GetSimpleRAG() *SimpleRAG {
	return s.rag
}

// getFilesFromDirectory returns all supported files from a directory
func getFilesFromDirectory(dirPath string) ([]string, error) {
	var files []string

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Check for supported file extensions
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".pdf", ".txt", ".md", ".docx", ".html", ".json":
			files = append(files, path)
		}

		return nil
	})

	return files, err
}
