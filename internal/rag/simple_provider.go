// Package rag provides a simple vector provider implementation using JSON storage
package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tmc/langchaingo/documentloaders"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/textsplitter"
)

// SimpleProvider implements VectorProvider using JSON file storage
type SimpleProvider struct {
	dbPath    string
	documents []SimpleDocument
}

// SimpleDocument represents a document chunk in the knowledge base
type SimpleDocument struct {
	ID       string            `json:"id"`
	Content  string            `json:"content"`
	Metadata map[string]string `json:"metadata"`
}

// DocumentScore represents a document with its relevance score
type DocumentScore struct {
	Document SimpleDocument
	Score    float64
}

// NewSimpleProvider creates a new simple vector provider
func NewSimpleProvider(dbPath string) *SimpleProvider {
	if dbPath == "" {
		dbPath = "./knowledge.json"
	}
	
	provider := &SimpleProvider{dbPath: dbPath}
	provider.load()
	return provider
}

// Initialize implements VectorProvider interface (no-op for simple provider)
func (s *SimpleProvider) Initialize(ctx context.Context) error {
	return nil
}

// IngestFile implements VectorProvider interface
func (s *SimpleProvider) IngestFile(ctx context.Context, filePath string, metadata map[string]string) (string, error) {
	// Only support PDF files for now
	if !strings.HasSuffix(strings.ToLower(filePath), ".pdf") {
		return "", fmt.Errorf("simple provider only supports PDF files, got: %s", filePath)
	}

	// Load PDF using LangChain Go  
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open PDF file: %w", err)
	}
	defer file.Close()

	loader := documentloaders.NewPDF(file, 0)
	docs, err := loader.Load(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to load PDF: %w", err)
	}

	if len(docs) == 0 {
		return "", fmt.Errorf("no content found in PDF")
	}

	// Split documents into chunks
	splitter := textsplitter.NewRecursiveCharacter(
		textsplitter.WithChunkSize(1000),
		textsplitter.WithChunkOverlap(200),
	)

	var allChunks []schema.Document
	for _, doc := range docs {
		chunks, err := splitter.SplitText(doc.PageContent)
		if err != nil {
			return "", fmt.Errorf("failed to split document: %w", err)
		}
		
		// Convert text chunks to schema.Document
		for i, chunk := range chunks {
			chunkDoc := schema.Document{
				PageContent: chunk,
				Metadata:    make(map[string]interface{}),
			}
			
			// Copy original metadata
			for k, v := range doc.Metadata {
				chunkDoc.Metadata[k] = v
			}
			
			// Add chunk index
			chunkDoc.Metadata["chunk_index"] = i
			
			allChunks = append(allChunks, chunkDoc)
		}
	}

	// Convert to our format and add to storage
	fileName := filepath.Base(filePath)
	fileID := fmt.Sprintf("file_%d", len(s.documents))
	
	for i, chunk := range allChunks {
		docMetadata := make(map[string]string)
		
		// Copy provided metadata
		for k, v := range metadata {
			docMetadata[k] = v
		}
		
		// Add file information
		docMetadata["file_name"] = fileName
		docMetadata["file_path"] = filePath
		docMetadata["chunk_index"] = fmt.Sprintf("%d", i)
		
		// Copy chunk metadata
		for k, v := range chunk.Metadata {
			if str, ok := v.(string); ok {
				docMetadata[k] = str
			} else {
				docMetadata[k] = fmt.Sprintf("%v", v)
			}
		}

		doc := SimpleDocument{
			ID:       fmt.Sprintf("%s_chunk_%d", fileID, i),
			Content:  chunk.PageContent,
			Metadata: docMetadata,
		}
		
		s.documents = append(s.documents, doc)
	}

	// Save to persistent storage
	if err := s.save(); err != nil {
		return "", fmt.Errorf("failed to save documents: %w", err)
	}

	return fileID, nil
}

// IngestFiles implements VectorProvider interface
func (s *SimpleProvider) IngestFiles(ctx context.Context, filePaths []string, metadata map[string]string) ([]string, error) {
	fileIDs := make([]string, 0, len(filePaths))
	
	for _, filePath := range filePaths {
		fileID, err := s.IngestFile(ctx, filePath, metadata)
		if err != nil {
			// Log error but continue with other files
			fmt.Printf("Warning: failed to ingest %s: %v\n", filePath, err)
			continue
		}
		fileIDs = append(fileIDs, fileID)
	}
	
	return fileIDs, nil
}

// DeleteFile implements VectorProvider interface
func (s *SimpleProvider) DeleteFile(ctx context.Context, fileID string) error {
	// Remove all documents with matching file ID
	var filteredDocs []SimpleDocument
	removed := 0
	
	for _, doc := range s.documents {
		if strings.HasPrefix(doc.ID, fileID+"_") {
			removed++
		} else {
			filteredDocs = append(filteredDocs, doc)
		}
	}
	
	if removed == 0 {
		return fmt.Errorf("file not found: %s", fileID)
	}
	
	s.documents = filteredDocs
	
	// Save changes
	if err := s.save(); err != nil {
		return fmt.Errorf("failed to save after deletion: %w", err)
	}
	
	return nil
}

// ListFiles implements VectorProvider interface
func (s *SimpleProvider) ListFiles(ctx context.Context, limit int) ([]FileInfo, error) {
	// Group documents by file
	fileMap := make(map[string]*FileInfo)
	
	for _, doc := range s.documents {
		fileName, ok := doc.Metadata["file_name"]
		if !ok {
			continue
		}
		
		filePath, _ := doc.Metadata["file_path"]
		
		if info, exists := fileMap[fileName]; exists {
			info.Size++ // Count chunks as size
		} else {
			fileMap[fileName] = &FileInfo{
				ID:       strings.Split(doc.ID, "_chunk_")[0],
				Name:     fileName,
				Size:     1,
				Metadata: map[string]string{"file_path": filePath},
				Status:   "completed",
			}
		}
	}
	
	// Convert to slice
	files := make([]FileInfo, 0, len(fileMap))
	for _, info := range fileMap {
		files = append(files, *info)
		if len(files) >= limit && limit > 0 {
			break
		}
	}
	
	return files, nil
}

// Search implements VectorProvider interface with improved text search
func (s *SimpleProvider) Search(ctx context.Context, query string, options SearchOptions) ([]SearchResult, error) {
	if len(s.documents) == 0 {
		return []SearchResult{}, nil
	}

	limit := options.Limit
	if limit <= 0 {
		limit = 10
	}

	// Calculate scores for all documents
	var scores []DocumentScore
	queryLower := strings.ToLower(query)
	queryTerms := strings.Fields(queryLower)

	for _, doc := range s.documents {
		contentLower := strings.ToLower(doc.Content)
		score := s.calculateRelevanceScore(contentLower, queryLower, queryTerms)
		
		if score > 0 {
			scores = append(scores, DocumentScore{
				Document: doc,
				Score:    score,
			})
		}
	}

	// Sort by score (descending)
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	// Limit results
	if len(scores) > limit {
		scores = scores[:limit]
	}

	// Convert to SearchResult format
	results := make([]SearchResult, len(scores))
	for i, scored := range scores {
		fileName, _ := scored.Document.Metadata["file_name"]
		fileID, _ := scored.Document.Metadata["file_path"]
		
		result := SearchResult{
			Content:    scored.Document.Content,
			Score:      float32(scored.Score),
			FileID:     fileID,
			FileName:   fileName,
			Metadata:   scored.Document.Metadata,
			Highlights: s.extractHighlights(scored.Document.Content, queryTerms),
		}
		
		results[i] = result
	}

	return results, nil
}

// GetStats implements VectorProvider interface
func (s *SimpleProvider) GetStats(ctx context.Context) (*VectorStoreStats, error) {
	files, err := s.ListFiles(ctx, 0) // Get all files
	if err != nil {
		return nil, err
	}
	
	stats := &VectorStoreStats{
		TotalFiles:  len(files),
		TotalChunks: len(s.documents),
		LastUpdated: time.Now(),
	}
	
	return stats, nil
}

// Close implements VectorProvider interface (no-op for simple provider)
func (s *SimpleProvider) Close() error {
	return nil
}

// calculateRelevanceScore computes a relevance score between query and content
func (s *SimpleProvider) calculateRelevanceScore(content, query string, queryTerms []string) float64 {
	if content == "" || query == "" {
		return 0
	}

	var score float64

	// Exact phrase match (highest weight)
	if strings.Contains(content, query) {
		score += 10.0
	}

	// Individual term matches
	contentWords := strings.Fields(content)
	contentWordSet := make(map[string]int)
	for _, word := range contentWords {
		contentWordSet[word]++
	}

	matchingTerms := 0
	for _, term := range queryTerms {
		if count, exists := contentWordSet[term]; exists {
			matchingTerms++
			// Term frequency component
			tf := float64(count) / float64(len(contentWords))
			score += tf * 5.0
		}
	}

	// Coverage bonus (how many query terms are matched)
	if len(queryTerms) > 0 {
		coverage := float64(matchingTerms) / float64(len(queryTerms))
		score += coverage * 3.0
	}

	// Partial word matches (lower weight)
	for _, term := range queryTerms {
		if len(term) > 3 {
			for _, word := range contentWords {
				if strings.Contains(word, term) && word != term {
					score += 0.5
				}
			}
		}
	}

	return score
}

// extractHighlights finds relevant snippets in content
func (s *SimpleProvider) extractHighlights(content string, queryTerms []string) []string {
	var highlights []string
	contentLower := strings.ToLower(content)
	
	for _, term := range queryTerms {
		if len(term) > 2 && strings.Contains(contentLower, term) {
			highlights = append(highlights, term)
		}
	}
	
	return highlights
}

// load reads documents from the JSON file
func (s *SimpleProvider) load() {
	if _, err := os.Stat(s.dbPath); os.IsNotExist(err) {
		s.documents = []SimpleDocument{}
		return
	}

	data, err := os.ReadFile(s.dbPath)
	if err != nil {
		fmt.Printf("Warning: failed to read RAG database: %v\n", err)
		s.documents = []SimpleDocument{}
		return
	}

	if err := json.Unmarshal(data, &s.documents); err != nil {
		fmt.Printf("Warning: failed to parse RAG database: %v\n", err)
		s.documents = []SimpleDocument{}
		return
	}
}

// save writes documents to the JSON file
func (s *SimpleProvider) save() error {
	// Ensure directory exists
	dir := filepath.Dir(s.dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.MarshalIndent(s.documents, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal documents: %w", err)
	}

	if err := os.WriteFile(s.dbPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// Register the simple provider
func init() {
	RegisterVectorProvider("simple", func(config map[string]interface{}) (VectorProvider, error) {
		dbPath := "./knowledge.json"
		if path, ok := config["database_path"].(string); ok && path != "" {
			dbPath = path
		}
		return NewSimpleProvider(dbPath), nil
	})
}