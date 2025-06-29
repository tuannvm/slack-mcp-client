// Package rag provides a simple RAG implementation using JSON storage
package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tmc/langchaingo/documentloaders"
	"github.com/tmc/langchaingo/textsplitter"
	"github.com/tuannvm/slack-mcp-client/internal/common"
	"github.com/tuannvm/slack-mcp-client/internal/llm"
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

// NewSimpleRAG creates a new SimpleRAG instance and loads existing data
func NewSimpleRAG(dbPath string) *SimpleRAG {
	rag := &SimpleRAG{dbPath: dbPath}
	rag.load()
	return rag
}

// Search performs simple text search - good enough to start
func (r *SimpleRAG) Search(query string, limit int) []Document {
	queryLower := strings.ToLower(query)
	queryWords := strings.Fields(queryLower) // Split into individual words
	var results []Document

	// If single word or exact phrase, use original logic
	if len(queryWords) == 1 {
		for _, doc := range r.documents {
			if strings.Contains(strings.ToLower(doc.Content), queryLower) {
				results = append(results, doc)
				if len(results) >= limit {
					break
				}
			}
		}
		return results
	}

	// For multi-word queries, use flexible matching
	type docScore struct {
		doc   Document
		score int
	}
	var scoredDocs []docScore

	for _, doc := range r.documents {
		docLower := strings.ToLower(doc.Content)
		score := 0

		// Score based on how many query words are found
		for _, word := range queryWords {
			if strings.Contains(docLower, word) {
				score++
			}
		}

		// Only include docs that match at least one word
		if score > 0 {
			scoredDocs = append(scoredDocs, docScore{doc: doc, score: score})
		}
	}

	// Sort by score (descending) and return top results
	for i := 0; i < len(scoredDocs)-1; i++ {
		for j := i + 1; j < len(scoredDocs); j++ {
			if scoredDocs[i].score < scoredDocs[j].score {
				scoredDocs[i], scoredDocs[j] = scoredDocs[j], scoredDocs[i]
			}
		}
	}

	// Extract documents up to limit
	for i, scored := range scoredDocs {
		if i >= limit {
			break
		}
		results = append(results, scored.doc)
	}

	return results
}

// IngestPDF processes a PDF file using existing LangChain patterns
func (r *SimpleRAG) IngestPDF(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open PDF file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			// Log the error but don't override the main function error
			fmt.Printf("Warning: failed to close file %s: %v\n", filePath, closeErr)
		}
	}()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	loader := documentloaders.NewPDF(file, info.Size())

	splitter := textsplitter.NewRecursiveCharacter(
		textsplitter.WithChunkSize(1000),
		textsplitter.WithChunkOverlap(200),
	)

	docs, err := loader.LoadAndSplit(context.Background(), splitter)
	if err != nil {
		return fmt.Errorf("failed to load and split PDF: %w", err)
	}

	// Convert to our format and append
	for i, doc := range docs {
		r.documents = append(r.documents, Document{
			Content: doc.PageContent,
			Metadata: map[string]string{
				"file_path":   filePath,
				"chunk_index": fmt.Sprintf("%d", i),
				"file_name":   filepath.Base(filePath),
			},
		})
	}

	return r.save()
}

// IngestDirectory processes all PDF files in a directory
func (r *SimpleRAG) IngestDirectory(dirPath string) (int, error) {
	count := 0
	err := filepath.Walk(dirPath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if filepath.Ext(filePath) == ".pdf" {
			if err := r.IngestPDF(filePath); err != nil {
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
func (r *SimpleRAG) GetDocumentCount() int {
	return len(r.documents)
}

// save writes the documents to the JSON file
func (r *SimpleRAG) save() error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(r.dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.MarshalIndent(r.documents, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal documents: %w", err)
	}

	return os.WriteFile(r.dbPath, data, 0644)
}

// load reads documents from the JSON file
func (r *SimpleRAG) load() {
	data, err := os.ReadFile(r.dbPath)
	if err != nil {
		// Start empty if file doesn't exist
		r.documents = []Document{}
		return
	}

	if err := json.Unmarshal(data, &r.documents); err != nil {
		// If unmarshal fails, start empty
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

			// Get limit parameter with default
			limit := req.GetInt("limit", 3)

			docs := r.Search(query, limit)

			// Build context for LLM
			var contextBuilder strings.Builder
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

			if len(docs) == 0 {
				contextBuilder.WriteString("No relevant context found in knowledge base.")
			}

			return llm.CreateMCPResult(contextBuilder.String()), nil
		},
	}
}

// AsToolInfo returns the tool information for MCP discovery
func (r *SimpleRAG) AsToolInfo() common.ToolInfo {
	return common.ToolInfo{
		ServerName:  "rag_search",
		Description: "Search knowledge base for relevant context",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Search query for knowledge base",
				},
				"limit": map[string]interface{}{
					"type":        "number",
					"description": "Maximum number of results to return (default: 3)",
					"default":     3,
				},
			},
			"required": []string{"query"},
		},
	}
}
