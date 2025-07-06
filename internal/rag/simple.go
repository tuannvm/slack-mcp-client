// Package rag provides a simple RAG implementation using JSON storage
package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tmc/langchaingo/documentloaders"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/textsplitter"
	"github.com/tmc/langchaingo/vectorstores"
	"github.com/tuannvm/slack-mcp-client/internal/llm"
	mcpinternal "github.com/tuannvm/slack-mcp-client/internal/mcp"
)

// SimpleRAG implements a basic RAG system using JSON file storage
type SimpleRAG struct {
	dbPath    string
	documents []Document
}

// Document represents a single document chunk in the knowledge base
type Document struct {
	Content  string            `json:"content"`
	Metadata map[string]string `json:"metadata"`
}

// DocumentScore represents a document with its relevance score
type DocumentScore struct {
	Document Document
	Score    float64
}

// NewSimpleRAG creates a new SimpleRAG instance and loads existing data
func NewSimpleRAG(dbPath string) *SimpleRAG {
	rag := &SimpleRAG{dbPath: dbPath}
	rag.load()
	return rag
}

// Search performs improved text search with better scoring
func (r *SimpleRAG) Search(query string, limit int) []Document {
	if len(r.documents) == 0 {
		return []Document{}
	}

	queryLower := strings.ToLower(strings.TrimSpace(query))
	if queryLower == "" {
		return []Document{}
	}

	queryWords := strings.Fields(queryLower)
	if len(queryWords) == 0 {
		return []Document{}
	}

	// Score all documents
	scoredDocs := make([]DocumentScore, 0, len(r.documents))

	for _, doc := range r.documents {
		score := r.calculateRelevanceScore(doc, queryWords)
		if score > 0 {
			scoredDocs = append(scoredDocs, DocumentScore{
				Document: doc,
				Score:    score,
			})
		}
	}

	// Sort by score descending using Go's built-in sort (O(n log n))
	sort.Slice(scoredDocs, func(i, j int) bool {
		return scoredDocs[i].Score > scoredDocs[j].Score
	})

	// Return top results
	maxResults := len(scoredDocs)
	if limit < maxResults {
		maxResults = limit
	}

	results := make([]Document, maxResults)
	for i := 0; i < maxResults; i++ {
		results[i] = scoredDocs[i].Document
	}

	return results
}

// calculateRelevanceScore computes a better relevance score
func (r *SimpleRAG) calculateRelevanceScore(doc Document, queryWords []string) float64 {
	content := strings.ToLower(doc.Content)
	fileName := strings.ToLower(doc.Metadata["file_name"])

	var score float64
	contentWords := strings.Fields(content)

	// Base scoring: word frequency with diminishing returns
	for _, queryWord := range queryWords {
		// Count occurrences in content
		contentMatches := strings.Count(content, queryWord)
		if contentMatches > 0 {
			// Use log to prevent over-weighting of repeated terms
			score += math.Log(float64(contentMatches) + 1.0)
		}

		// Boost score if query word appears in filename
		if strings.Contains(fileName, queryWord) {
			score += 2.0
		}

		// Boost score for exact phrase matches
		if len(queryWords) > 1 && strings.Contains(content, strings.Join(queryWords, " ")) {
			score += 3.0
		}
	}

	// Normalize by document length to prevent bias toward longer docs
	if len(contentWords) > 0 {
		score = score / math.Log(float64(len(contentWords))+1.0)
	}

	return score
}

// IngestPDFSimple processes a PDF file using existing LangChain patterns (for backward compatibility)
func (r *SimpleRAG) IngestPDFSimple(filePath string) error {
	return r.IngestPDF(context.Background(), filePath)
}

// IngestPDF implements RAGProvider interface
func (r *SimpleRAG) IngestPDF(ctx context.Context, filePath string, options ...IngestOption) error {
	if filePath == "" {
		return fmt.Errorf("file path cannot be empty")
	}

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open PDF file %s: %w", filePath, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			// Log the error but don't override the main function error
			fmt.Printf("Warning: failed to close file %s: %v\n", filePath, closeErr)
		}
	}()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info for %s: %w", filePath, err)
	}

	loader := documentloaders.NewPDF(file, info.Size())

	splitter := textsplitter.NewRecursiveCharacter(
		textsplitter.WithChunkSize(1000),
		textsplitter.WithChunkOverlap(200),
	)

	docs, err := loader.LoadAndSplit(context.Background(), splitter)
	if err != nil {
		return fmt.Errorf("failed to load and split PDF %s: %w", filePath, err)
	}

	// Convert to our format and append
	for i, doc := range docs {
		r.documents = append(r.documents, Document{
			Content: doc.PageContent,
			Metadata: map[string]string{
				"file_path":   filePath,
				"chunk_index": fmt.Sprintf("%d", i),
				"file_name":   filepath.Base(filePath),
				"file_type":   "pdf",
			},
		})
	}

	return r.save()
}

// IngestDirectorySimple processes all PDF files in a directory (for backward compatibility)
func (r *SimpleRAG) IngestDirectorySimple(dirPath string) (int, error) {
	return r.IngestDirectory(context.Background(), dirPath)
}

// IngestDirectory implements RAGProvider interface
func (r *SimpleRAG) IngestDirectory(ctx context.Context, dirPath string, options ...IngestOption) (int, error) {
	if dirPath == "" {
		return 0, fmt.Errorf("directory path cannot be empty")
	}

	count := 0
	err := filepath.Walk(dirPath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error walking path %s: %w", filePath, err)
		}

		if strings.ToLower(filepath.Ext(filePath)) == ".pdf" {
			if err := r.IngestPDF(ctx, filePath, options...); err != nil {
				return fmt.Errorf("failed to ingest %s: %w", filePath, err)
			}
			count++
		}
		return nil
	})

	if err != nil {
		return count, err
	}

	return count, nil
}

// GetDocumentCount returns the total number of documents in the knowledge base
// This is the original method for backward compatibility
func (r *SimpleRAG) GetDocumentCountSimple() int {
	return len(r.documents)
}

// GetDocumentCount implements RAGProvider interface
func (r *SimpleRAG) GetDocumentCount(ctx context.Context) (int, error) {
	return len(r.documents), nil
}

// save writes the documents to the JSON file
func (r *SimpleRAG) save() error {
	if r.dbPath == "" {
		return fmt.Errorf("database path not set")
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(r.dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(r.documents, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal documents: %w", err)
	}

	if err := os.WriteFile(r.dbPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write to %s: %w", r.dbPath, err)
	}

	return nil
}

// load reads documents from the JSON file
func (r *SimpleRAG) load() {
	if r.dbPath == "" {
		r.documents = []Document{}
		return
	}

	data, err := os.ReadFile(r.dbPath)
	if err != nil {
		// Start empty if file doesn't exist
		r.documents = []Document{}
		return
	}

	if err := json.Unmarshal(data, &r.documents); err != nil {
		// If unmarshal fails, start empty but log the error
		fmt.Printf("Warning: failed to unmarshal documents from %s: %v\n", r.dbPath, err)
		r.documents = []Document{}
	}
}

// AsMCPHandler converts the RAG system to an MCP tool handler
func (r *SimpleRAG) AsMCPHandler() *llm.MCPHandler {
	return &llm.MCPHandler{
		Name:        "rag_search",
		Description: "Search knowledge base for relevant context",
		HandleFunc: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query, err := req.RequireString("query")
			if err != nil {
				return nil, fmt.Errorf("query parameter required: %w", err)
			}

			if strings.TrimSpace(query) == "" {
				return nil, fmt.Errorf("query cannot be empty")
			}

			// Get limit parameter with default
			limit := req.GetInt("limit", 3)
			if limit < 1 || limit > 20 {
				limit = 3 // Reasonable bounds
			}

			docs := r.Search(query, limit)

			// Build context for LLM
			var contextBuilder strings.Builder

			if len(docs) == 0 {
				contextBuilder.WriteString("No relevant context found in knowledge base.")
			} else {
				contextBuilder.WriteString("Found relevant context:\n\n")
				for i, doc := range docs {
					contextBuilder.WriteString(fmt.Sprintf("Context %d", i+1))
					if fileName, ok := doc.Metadata["file_name"]; ok {
						contextBuilder.WriteString(fmt.Sprintf(" (from %s)", fileName))
					}
					contextBuilder.WriteString(":\n")
					contextBuilder.WriteString(doc.Content)
					contextBuilder.WriteString("\n\n")
				}
			}

			return llm.CreateMCPResult(contextBuilder.String()), nil
		},
	}
}

// AsToolInfo returns the tool information for MCP discovery
func (r *SimpleRAG) AsToolInfo() mcpinternal.ToolInfo {
	return mcpinternal.ToolInfo{
		ServerName:      "rag_search",
		ToolDescription: "Search knowledge base for relevant context",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Search query for knowledge base",
				},
				"limit": map[string]interface{}{
					"type":        "number",
					"description": "Maximum number of results to return (default: 3, max: 20)",
					"default":     3,
					"minimum":     1,
					"maximum":     20,
				},
			},
			"required": []string{"query"},
		},
	}
}

// Implement RAGProvider interface methods for SimpleRAG

// AddDocuments implements the LangChain Go VectorStore interface
func (r *SimpleRAG) AddDocuments(ctx context.Context, docs []schema.Document, options ...vectorstores.Option) ([]string, error) {
	ids := make([]string, len(docs))

	for i, doc := range docs {
		// Convert metadata
		metadata := make(map[string]string)
		for k, v := range doc.Metadata {
			if str, ok := v.(string); ok {
				metadata[k] = str
			} else {
				metadata[k] = fmt.Sprintf("%v", v)
			}
		}

		// Add document
		r.documents = append(r.documents, Document{
			Content:  doc.PageContent,
			Metadata: metadata,
		})
		ids[i] = fmt.Sprintf("doc_%d", len(r.documents))
	}

	return ids, r.save()
}

// SimilaritySearch implements the LangChain Go VectorStore interface
func (r *SimpleRAG) SimilaritySearch(ctx context.Context, query string, numDocuments int, options ...vectorstores.Option) ([]schema.Document, error) {
	results := r.Search(query, numDocuments)

	// Convert to schema.Document
	docs := make([]schema.Document, len(results))
	for i, result := range results {
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

// DeleteDocuments implements RAGManager interface
func (r *SimpleRAG) DeleteDocuments(ctx context.Context, ids []string) error {
	// SimpleRAG doesn't track IDs, so this is a no-op
	return fmt.Errorf("delete documents not implemented for SimpleRAG")
}

// GetStats implements RAGManager interface
func (r *SimpleRAG) GetStats(ctx context.Context) (*RAGStats, error) {
	count := r.GetDocumentCountSimple()
	return &RAGStats{
		DocumentCount: int64(count),
		FileTypeCounts: map[string]int{
			"pdf": count, // Assume all are PDFs for now
		},
	}, nil
}

// Close implements RAGManager interface
func (r *SimpleRAG) Close() error {
	// SimpleRAG doesn't need cleanup
	return nil
}
