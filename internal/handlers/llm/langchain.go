// Package llm provides handlers for integrating with language model providers
package llm

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"

	customErrors "github.com/tuannvm/slack-mcp-client/internal/common/errors"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
	"github.com/tuannvm/slack-mcp-client/internal/handlers"
)

// LangChainHandler implements the OpenAI tool using LangChainGo
type LangChainHandler struct {
	handlers.BaseHandler
	llm          llms.LLM
	defaultModel string
	apiEndpoint  string
}

// NewLangChainHandler creates a new LangChainHandler
func NewLangChainHandler(logger *logging.Logger) *LangChainHandler {
	// Get API key from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	apiEndpoint := os.Getenv("OPENAI_API_ENDPOINT")
	defaultModel := os.Getenv("OPENAI_MODEL")

	if defaultModel == "" {
		defaultModel = "gpt-4o"
		logger.Info("No OPENAI_MODEL specified, defaulting to %s", defaultModel)
	}

	// Initialize OpenAI client from LangChainGo
	opts := []openai.Option{
		openai.WithToken(apiKey),
	}

	// If custom API endpoint is set, use it
	if apiEndpoint != "" {
		opts = append(opts, openai.WithBaseURL(apiEndpoint))
	}

	llm, err := openai.New(opts...)
	if err != nil {
		logger.Error("Failed to initialize LangChainGo OpenAI client: %v", err)
		// We'll continue with a nil client and check for it in IsConfigured()
	}

	// Create tool definition
	tool := mcp.NewTool(
		"langchain",
		mcp.WithDescription("Process text using LangChainGo with OpenAI models"),
		mcp.WithString("model",
			mcp.Description("The OpenAI model to use"),
			mcp.Required(),
		),
		mcp.WithString("prompt",
			mcp.Description("The prompt to send to OpenAI (alternative to messages)"),
		),
		mcp.WithArray("messages",
			mcp.Description("Array of messages to send to OpenAI (alternative to prompt)"),
		),
		mcp.WithNumber("temperature",
			mcp.Description("Temperature for response generation"),
		),
		mcp.WithNumber("max_tokens",
			mcp.Description("Maximum number of tokens to generate"),
		),
	)

	return &LangChainHandler{
		BaseHandler: handlers.BaseHandler{
			Name:        "langchain",
			Description: "Process text using LangChainGo with OpenAI models",
			Tool:        tool,
			Logger:      logger.WithName("langchain-tool"),
		},
		llm:          llm,
		defaultModel: defaultModel,
		apiEndpoint:  apiEndpoint,
	}
}

// Handle processes a LangChain tool request
func (h *LangChainHandler) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	h.Logger.Debug("Received LangChain tool request")

	// Check if LLM client is configured
	if h.llm == nil {
		return nil, customErrors.NewOpenAIError("LangChainGo client not initialized", "missing_client", nil)
	}

	// Extract parameters from request
	args := request.Params.Arguments
	h.Logger.Debug("Received arguments: %+v", args)

	// Get model parameter with fallback to default
	model, ok := args["model"].(string)
	if !ok || model == "" {
		model = h.defaultModel
		h.Logger.Debug("Using default model: %s", model)
	}

	// Process input (either prompt or messages)
	prompt, messages := h.processInput(args)

	// Apply options for the LLM call
	options := h.buildOptions(args, model)

	// Call the LLM
	var result string
	var err error

	if len(messages) > 0 {
		// Use the messages for chat completion
		result, err = h.callLLMWithMessages(ctx, messages, options)
	} else {
		// Use the single prompt
		result, err = h.callLLMWithPrompt(ctx, prompt, options)
	}

	if err != nil {
		return nil, err
	}

	// Create and return MCP tool response
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: result,
			},
		},
	}, nil
}

// processInput extracts either a prompt string or structured messages from the arguments
func (h *LangChainHandler) processInput(args map[string]interface{}) (string, []map[string]string) {
	var prompt string
	var messages []map[string]string

	// Check if we have a simple prompt
	if p, ok := args["prompt"].(string); ok && p != "" {
		prompt = p
	}

	// Check for message array (takes precedence over prompt if both exist)
	if rawMessages, ok := args["messages"].([]interface{}); ok && len(rawMessages) > 0 {
		for _, rawMsg := range rawMessages {
			if msgMap, ok := rawMsg.(map[string]interface{}); ok {
				role, _ := msgMap["role"].(string)
				content, _ := msgMap["content"].(string)

				if role != "" && content != "" {
					messages = append(messages, map[string]string{
						"role":    role,
						"content": content,
					})
				}
			}
		}
	}

	return prompt, messages
}

// buildOptions creates options for the LLM call
func (h *LangChainHandler) buildOptions(args map[string]interface{}, model string) []llms.CallOption {
	var options []llms.CallOption

	// Set the model
	options = append(options, llms.WithModel(model))

	// Handle temperature
	if temp, ok := args["temperature"].(float64); ok {
		options = append(options, llms.WithTemperature(temp))
	}

	// Handle token limits
	if maxTokens, ok := args["max_tokens"].(float64); ok {
		options = append(options, llms.WithMaxTokens(int(maxTokens)))
	}

	return options
}

// callLLMWithPrompt sends a single prompt to the LLM
func (h *LangChainHandler) callLLMWithPrompt(ctx context.Context, prompt string, options []llms.CallOption) (string, error) {
	h.Logger.Debug("Calling LangChainGo OpenAI with prompt: %s", prompt)

	// Call the LLM with the prompt
	completion, err := h.llm.Call(ctx, prompt, options...)
	if err != nil {
		h.Logger.Error("LangChainGo OpenAI request failed: %v", err)
		return "", customErrors.NewOpenAIError(
			fmt.Sprintf("LangChainGo API request failed: %v", err),
			"request_failed",
			err,
		)
	}

	h.Logger.Debug("Received response of length %d", len(completion))
	return completion, nil
}

// callLLMWithMessages sends structured messages to the LLM
func (h *LangChainHandler) callLLMWithMessages(ctx context.Context, messages []map[string]string, options []llms.CallOption) (string, error) {
	h.Logger.Debug("Calling LangChainGo OpenAI with %d messages", len(messages))

	// Convert our simple message format to a prompt
	var promptBuilder strings.Builder

	for _, msg := range messages {
		role := msg["role"]
		content := msg["content"]

		promptBuilder.WriteString(fmt.Sprintf("%s: %s\n", strings.ToUpper(role), content))
	}

	prompt := promptBuilder.String()

	// Add one final assistant prefix to indicate where the response should go
	prompt += "ASSISTANT: "

	// Call the LLM with the constructed prompt
	return h.callLLMWithPrompt(ctx, prompt, options)
}

// IsConfigured returns true if the handler is properly configured
func (h *LangChainHandler) IsConfigured() bool {
	return h.llm != nil
}
