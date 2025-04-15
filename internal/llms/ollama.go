package llms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
)

var logger = log.New(os.Stdout, "ollama-tool: ", log.LstdFlags)

// Default Ollama host if not specified in environment
var ollamaHost = "http://localhost:11434"

// GenerateRequest represents a request to the Ollama API
type GenerateRequest struct {
	Model       string                 `json:"model"`
	Prompt      string                 `json:"prompt"`
	Options     map[string]interface{} `json:"options,omitempty"`
	System      string                 `json:"system,omitempty"`
	Template    string                 `json:"template,omitempty"`
	Context     []int                  `json:"context,omitempty"`
	Stream      bool                   `json:"stream,omitempty"`
	Raw         bool                   `json:"raw,omitempty"`
	Keep_alive  string                 `json:"keep_alive,omitempty"`
}

// GenerateResponse represents a response from the Ollama API
type GenerateResponse struct {
	Model              string  `json:"model"`
	CreatedAt          string  `json:"created_at"`
	Response           string  `json:"response"`
	Done               bool    `json:"done"`
	Context            []int   `json:"context,omitempty"`
	TotalDuration      int64   `json:"total_duration,omitempty"`
	LoadDuration       int64   `json:"load_duration,omitempty"`
	PromptEvalCount    int     `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64   `json:"prompt_eval_duration,omitempty"`
	EvalCount          int     `json:"eval_count,omitempty"`
	EvalDuration       int64   `json:"eval_duration,omitempty"`
}

func init() {
	// Check for OLLAMA_HOST environment variable
	if envHost := os.Getenv("OLLAMA_HOST"); envHost != "" {
		ollamaHost = envHost
	}
	logger.Printf("Using Ollama host: %s", ollamaHost)
}

// HandleOllamaTool processes requests to the Ollama tool
func HandleOllamaTool(ctx context.Context, req mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	logger.Printf("Received Ollama tool request: %+v", req)

	// Extract parameters from request
	args := req.Params.Arguments
	logger.Printf("Received arguments: %+v", args)

	model, ok := args["model"].(string)
	if !ok {
		logger.Printf("Invalid model type: %T", args["model"])
		return nil, fmt.Errorf("model parameter is required and must be a string")
	}

	prompt, ok := args["prompt"].(string)
	if !ok {
		logger.Printf("Invalid prompt type: %T", args["prompt"])
		return nil, fmt.Errorf("prompt parameter is required and must be a string")
	}

	logger.Printf("Using model: %s with prompt: %s", model, prompt)

	// Optional parameters
	options := map[string]interface{}{}

	// Handle temperature parameter (can be string or float64)
	if temp, ok := args["temperature"]; ok {
		switch v := temp.(type) {
		case string:
			tempFloat, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid temperature value: %v", v)
			}
			options["temperature"] = tempFloat
		case float64:
			options["temperature"] = v
		default:
			return nil, fmt.Errorf("temperature must be a number or string")
		}
	}

	// Handle max_tokens parameter (can be string or float64)
	if maxTokens, ok := args["max_tokens"]; ok {
		switch v := maxTokens.(type) {
		case string:
			maxTokensInt, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("invalid max_tokens value: %v", v)
			}
			options["num_predict"] = maxTokensInt
		case float64:
			options["num_predict"] = int(v)
		default:
			return nil, fmt.Errorf("max_tokens must be a number or string")
		}
	}

	// Create the request payload
	generateReq := GenerateRequest{
		Model:   model,
		Prompt:  prompt,
		Options: options,
		Stream:  false,
	}

	// Convert request to JSON
	reqBody, err := json.Marshal(generateReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequest("POST", ollamaHost+"/api/generate", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Send request
	logger.Printf("Sending request to Ollama at %s", ollamaHost)
	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		logger.Printf("Failed to send request: %v", err)
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Printf("Failed to read response: %v", err)
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		logger.Printf("Ollama error response: %s", string(respBody))
		return nil, fmt.Errorf("Ollama returned error (Status: %s): %s", resp.Status, string(respBody))
	}

	logger.Printf("Received successful response from Ollama")

	// Parse response
	var generateResp GenerateResponse
	if err := json.Unmarshal(respBody, &generateResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Create MCP tool response with the same structure as the Hello tool
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: generateResp.Response,
			},
		},
	}, nil
}