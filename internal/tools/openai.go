package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

var openaiLogger = log.New(os.Stdout, "openai-tool: ", log.LstdFlags)

// Default OpenAI API endpoint
var openaiAPIEndpoint = "https://api.openai.com/v1/chat/completions"

// OpenAIMessage represents a message in the OpenAI API request
type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenAICompletionRequest represents a request to the OpenAI API
type OpenAICompletionRequest struct {
	Model       string                 `json:"model"`
	Messages    []OpenAIMessage        `json:"messages,omitempty"`
	Prompt      string                 `json:"prompt,omitempty"`
	MaxTokens   int                    `json:"max_tokens,omitempty"`
	Temperature float64                `json:"temperature,omitempty"`
	Stream      bool                   `json:"stream,omitempty"`
	Options     map[string]interface{} `json:"options,omitempty"`
}

// OpenAIChatRequest represents a request to the OpenAI chat completions API
type OpenAIChatRequest struct {
	Model       string         `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	Temperature float64        `json:"temperature,omitempty"`
	MaxTokens   int            `json:"max_tokens,omitempty"`
	Stream      bool           `json:"stream,omitempty"`
}

// OpenAIChatResponseChoice represents a choice in the OpenAI API response
type OpenAIChatResponseChoice struct {
	Index        int          `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string       `json:"finish_reason"`
}

// OpenAIChatResponseUsage represents usage statistics in the OpenAI API response
type OpenAIChatResponseUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAIChatResponse represents a response from the OpenAI chat completions API
type OpenAIChatResponse struct {
	ID      string                   `json:"id"`
	Object  string                   `json:"object"`
	Created int64                    `json:"created"`
	Model   string                   `json:"model"`
	Choices []OpenAIChatResponseChoice `json:"choices"`
	Usage   OpenAIChatResponseUsage  `json:"usage"`
}

// OpenAIApiKey holds the API key for OpenAI
var openaiApiKey string

func init() {
	// Check for OPENAI_API_KEY environment variable
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		openaiApiKey = apiKey
		openaiLogger.Printf("OpenAI API key configured")
	} else {
		openaiLogger.Printf("Warning: OPENAI_API_KEY not set, OpenAI tool will not function")
	}

	// Check for custom API endpoint
	if endpoint := os.Getenv("OPENAI_API_ENDPOINT"); endpoint != "" {
		openaiAPIEndpoint = endpoint
		openaiLogger.Printf("Using custom OpenAI API endpoint: %s", openaiAPIEndpoint)
	}
}

// HandleOpenAITool handles requests to the OpenAI API
func HandleOpenAIToolLegacy(params map[string]interface{}) (string, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY environment variable not set")
	}

	// Extract required parameters
	model, ok := params["model"].(string)
	if !ok || model == "" {
		model = "gpt-4"
	}

	// Create request body
	reqBody := OpenAICompletionRequest{
		Model:       model,
		Temperature: 0.7,
		MaxTokens:   2048,
	}

	// Handle different input formats
	if prompt, ok := params["prompt"].(string); ok && prompt != "" {
		// If it's a text completion request
		if model == "gpt-3.5-turbo" || model == "gpt-4" || model == "gpt-4-turbo" {
			// For chat models, convert prompt to messages
			reqBody.Messages = []OpenAIMessage{
				{
					Role:    "user",
					Content: prompt,
				},
			}
		} else {
			// For completion models
			reqBody.Prompt = prompt
		}
	} else if msgs, ok := params["messages"].([]interface{}); ok {
		// If it's a chat completion with messages array
		messages := make([]OpenAIMessage, 0, len(msgs))
		for _, msg := range msgs {
			if msgMap, ok := msg.(map[string]interface{}); ok {
				role, _ := msgMap["role"].(string)
				content, _ := msgMap["content"].(string)
				if role != "" && content != "" {
					messages = append(messages, OpenAIMessage{
						Role:    role,
						Content: content,
					})
				}
			}
		}
		reqBody.Messages = messages
	} else {
		return "", fmt.Errorf("either 'prompt' or 'messages' parameter is required")
	}

	// Apply optional parameters
	if maxTokens, ok := params["max_tokens"].(float64); ok {
		reqBody.MaxTokens = int(maxTokens)
	}
	if temperature, ok := params["temperature"].(float64); ok {
		reqBody.Temperature = temperature
	}
	if options, ok := params["options"].(map[string]interface{}); ok {
		reqBody.Options = options
	}

	// Determine the API endpoint based on model
	endpoint := "https://api.openai.com/v1/chat/completions"
	if !(model == "gpt-3.5-turbo" || model == "gpt-4" || model == "gpt-4-turbo") {
		endpoint = "https://api.openai.com/v1/completions"
	}

	// Create and send the request
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}

	client := &http.Client{
		Timeout: 120 * time.Second,
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Read and parse the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenAI API error: %s", body)
	}

	// Parse the response
	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	// Extract the result based on the response format
	var result string
	if choices, ok := response["choices"].([]interface{}); ok && len(choices) > 0 {
		choice := choices[0].(map[string]interface{})
		if message, ok := choice["message"].(map[string]interface{}); ok {
			if content, ok := message["content"].(string); ok {
				result = content
			}
		} else if text, ok := choice["text"].(string); ok {
			result = text
		} else {
			return "", fmt.Errorf("unexpected response format from OpenAI API")
		}
	} else {
		return "", fmt.Errorf("no choices in OpenAI API response")
	}

	return result, nil
}

// HandleOpenAITool processes requests to the OpenAI tool
func HandleOpenAITool(ctx context.Context, req mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	logger.Printf("Received OpenAI tool request: %+v", req)

	// Check if API key is configured
	if openaiApiKey == "" {
		return nil, fmt.Errorf("OpenAI API key not configured. Set OPENAI_API_KEY environment variable")
	}

	// Extract parameters from request
	args := req.Params.Arguments
	logger.Printf("Received arguments: %+v", args)

	// Get required parameters
	model, ok := args["model"].(string)
	if !ok {
		return nil, fmt.Errorf("model parameter is required and must be a string")
	}

	// Handle messages parameter
	var messages []OpenAIMessage
	
	// First, check if there's a simple "prompt" parameter as a shortcut
	if prompt, ok := args["prompt"].(string); ok {
		messages = []OpenAIMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		}
	} else if rawMessages, ok := args["messages"].([]interface{}); ok {
		// Process array of message objects
		for _, rawMsg := range rawMessages {
			if msgMap, ok := rawMsg.(map[string]interface{}); ok {
				role, _ := msgMap["role"].(string)
				content, _ := msgMap["content"].(string)
				
				if role == "" || content == "" {
					return nil, fmt.Errorf("messages must contain role and content fields")
				}
				
				messages = append(messages, OpenAIMessage{
					Role:    role,
					Content: content,
				})
			} else {
				return nil, fmt.Errorf("invalid message format in messages array")
			}
		}
	} else {
		return nil, fmt.Errorf("either prompt or messages parameter is required")
	}

	// Build the request
	chatReq := OpenAIChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
	}

	// Optional parameters
	if temp, ok := args["temperature"].(float64); ok {
		chatReq.Temperature = temp
	}

	if maxTokens, ok := args["max_tokens"].(float64); ok {
		chatReq.MaxTokens = int(maxTokens)
	}

	// Convert request to JSON
	reqBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", openaiAPIEndpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+openaiApiKey)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	// Send request
	logger.Printf("Sending request to OpenAI API: %s", openaiAPIEndpoint)
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
		logger.Printf("OpenAI error response (Status %d): %s", resp.StatusCode, string(respBody))
		return nil, fmt.Errorf("OpenAI API returned error (Status: %s): %s", resp.Status, string(respBody))
	}

	logger.Printf("Received successful response from OpenAI")

	// Parse response
	var chatResp OpenAIChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check if we have choices in the response
	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("OpenAI API returned no choices in response")
	}

	// Extract the message content
	aiResponse := chatResp.Choices[0].Message.Content

	// Create MCP tool response
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: aiResponse,
			},
		},
	}, nil
} 