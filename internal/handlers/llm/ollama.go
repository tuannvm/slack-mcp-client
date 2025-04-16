// Package llm provides handlers for integrating with language model providers
package llm

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
	customErrors "github.com/tuannvm/slack-mcp-client/internal/common/errors"
	httpClient "github.com/tuannvm/slack-mcp-client/internal/common/http"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
	"github.com/tuannvm/slack-mcp-client/internal/handlers"
)

// GenerateRequest represents a request to the Ollama API
type GenerateRequest struct {
	Model     string                 `json:"model"`
	Prompt    string                 `json:"prompt"`
	Options   map[string]interface{} `json:"options,omitempty"`
	System    string                 `json:"system,omitempty"`
	Template  string                 `json:"template,omitempty"`
	Context   []int                  `json:"context,omitempty"`
	Stream    bool                   `json:"stream,omitempty"`
	Raw       bool                   `json:"raw,omitempty"`
	KeepAlive string                 `json:"keep_alive,omitempty"`
}

// GenerateResponse represents a response from the Ollama API
type GenerateResponse struct {
	Model              string `json:"model"`
	CreatedAt          string `json:"created_at"`
	Response           string `json:"response"`
	Done               bool   `json:"done"`
	Context            []int  `json:"context,omitempty"`
	TotalDuration      int64  `json:"total_duration,omitempty"`
	LoadDuration       int64  `json:"load_duration,omitempty"`
	PromptEvalCount    int    `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64  `json:"prompt_eval_duration,omitempty"`
	EvalCount          int    `json:"eval_count,omitempty"`
	EvalDuration       int64  `json:"eval_duration,omitempty"`
}

// OllamaHandler implements the Ollama tool
type OllamaHandler struct {
	handlers.BaseHandler
	apiEndpoint string
	httpClient  *httpClient.Client
}

// NewOllamaHandler creates a new OllamaHandler
func NewOllamaHandler(logger *logging.Logger) *OllamaHandler {
	// Get API endpoint from environment
	apiEndpoint := os.Getenv("OLLAMA_API_ENDPOINT")

	if apiEndpoint == "" {
		apiEndpoint = "http://localhost:11434"
	}

	// Set up HTTP client with logging
	options := httpClient.DefaultOptions()
	options.Timeout = 120 * 1000000000 // 120 seconds for Ollama, as it can be slower
	options.RequestLogger = func(_, url string, _ []byte) {
		logger.Debug("Ollama API Request: %s", url)
	}
	options.ResponseLogger = func(statusCode int, _ []byte, err error) {
		if err != nil {
			logger.Error("Ollama API Response error: %v", err)
			return
		}
		logger.Debug("Ollama API Response: status=%d", statusCode)
	}

	// Create tool definition
	tool := mcp.NewTool(
		"ollama",
		mcp.WithDescription("Process text using Ollama LLM"),
		mcp.WithString("model",
			mcp.Description("The Ollama model to use"),
			mcp.Required(),
		),
		mcp.WithString("prompt",
			mcp.Description("The prompt to send to Ollama"),
			mcp.Required(),
		),
		mcp.WithString("temperature",
			mcp.Description("Temperature for response generation"),
		),
		mcp.WithString("max_tokens",
			mcp.Description("Maximum number of tokens to generate"),
		),
	)

	return &OllamaHandler{
		BaseHandler: handlers.BaseHandler{
			Name:        "ollama",
			Description: "Process text using Ollama LLM",
			Tool:        tool,
			Logger:      logger.WithName("ollama-tool"),
		},
		apiEndpoint: apiEndpoint,
		httpClient:  httpClient.NewClient(options),
	}
}

// Handle processes an Ollama tool request
func (h *OllamaHandler) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	h.Logger.Debug("Received Ollama tool request")

	// Extract parameters from request
	args := request.Params.Arguments
	h.Logger.Debug("Received arguments: %+v", args)

	model, ok := args["model"].(string)
	if !ok {
		h.Logger.Error("Invalid model type: %T", args["model"])
		return nil, customErrors.ErrBadRequest
	}

	prompt, ok := args["prompt"].(string)
	if !ok {
		h.Logger.Error("Invalid prompt type: %T", args["prompt"])
		return nil, customErrors.ErrBadRequest
	}

	h.Logger.Debug("Using model: %s with prompt: %s", model, prompt)

	// Optional parameters
	options := map[string]interface{}{}

	// Handle temperature parameter (can be string or float64)
	if temp, ok := args["temperature"]; ok {
		switch v := temp.(type) {
		case string:
			tempFloat, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, customErrors.ErrBadRequest
			}
			options["temperature"] = tempFloat
		case float64:
			options["temperature"] = v
		default:
			return nil, customErrors.ErrBadRequest
		}
	}

	// Handle max_tokens parameter (can be string or float64)
	if maxTokens, ok := args["max_tokens"]; ok {
		switch v := maxTokens.(type) {
		case string:
			maxTokensInt, err := strconv.Atoi(v)
			if err != nil {
				return nil, customErrors.ErrBadRequest
			}
			options["num_predict"] = maxTokensInt
		case float64:
			options["num_predict"] = int(v)
		default:
			return nil, customErrors.ErrBadRequest
		}
	}

	// Create the request payload
	generateReq := GenerateRequest{
		Model:   model,
		Prompt:  prompt,
		Options: options,
		Stream:  false,
	}

	// Prepare headers
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	// Make the request
	var generateResp GenerateResponse
	statusCode, err := h.httpClient.DoJSONRequest(
		ctx,
		"POST",
		h.apiEndpoint+"/api/generate",
		generateReq,
		&generateResp,
		headers,
	)

	if err != nil {
		h.Logger.Error("Ollama API request failed: %v", err)
		if customErrors.Is(err, customErrors.ErrTooManyRequests) {
			return nil, customErrors.NewOllamaError("Rate limit exceeded", "rate_limited", err)
		}
		return nil, customErrors.NewOllamaError("API request failed", "request_failed", err)
	}

	if statusCode != 200 {
		return nil, customErrors.NewOllamaError(
			fmt.Sprintf("API returned error status: %d", statusCode),
			"api_error",
			customErrors.StatusCodeToError(statusCode),
		)
	}

	h.Logger.Debug("Received response from Ollama: %s", generateResp.Model)

	// Create MCP tool response
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: generateResp.Response,
			},
		},
	}, nil
}

// IsConfigured returns true if the handler is properly configured
func (h *OllamaHandler) IsConfigured() bool {
	return true // Ollama just needs an endpoint, which has a default
}
