package slackbot

import (
	"context"
	"encoding/json"
	"github.com/tmc/langchaingo/callbacks"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
	"regexp"
	"strings"

	"github.com/slack-go/slack"
)

type sendMessageFunc func(message string)

type agentCallbackHandler struct {
	callbacks.SimpleHandler
	sendMessage sendMessageFunc
	logger      *logging.Logger
}

func (handler *agentCallbackHandler) HandleChainEnd(_ context.Context, outputs map[string]any) {
	if text, ok := outputs["text"]; ok {
		if textStr, ok := text.(string); ok {
			if isThinkingMessage(textStr) {
				textStr = formatContextMessageBlock(textStr, handler.logger)
			} else {
				textStr = formatFinalResponse(textStr)
			}
			handler.sendMessage(textStr)
		}
	}
}

var thinkingPattern = regexp.MustCompile(`Do I need to use a tool\? Yes`)

func isThinkingMessage(msg string) bool {
	return thinkingPattern.MatchString(msg)
}

// formatFinalResponse removes LLM agent response prefixes.
// The agent response format is defined in internal/llm/langchain.go
// > Thought: Do I need to use a tool? No
// > AI: [your response here]
func formatFinalResponse(msg string) string {
	msg = strings.Replace(msg, "Do I need to use a tool? No", "", 1)
	msg = strings.Replace(msg, "AI:", "", 1)
	return strings.TrimSpace(msg)
}

func formatContextMessageBlock(message string, logger *logging.Logger) string {
	mrkdwnBlock := slack.NewTextBlockObject("mrkdwn", message, false, false)
	contextBlock := slack.NewContextBlock("", []slack.MixedElement{mrkdwnBlock}...)
	blockMessage := slack.NewBlockMessage(contextBlock)

	jsonByte, err := json.Marshal(blockMessage)
	if err != nil {
		// Fallback to plain message if marshaling fails
		logger.ErrorKV("Failed to marshal block message", "error", err)
		return message
	}
	return string(jsonByte)
}
