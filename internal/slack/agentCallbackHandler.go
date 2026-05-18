package slackbot

import (
	"context"
	"strings"

	"github.com/tmc/langchaingo/callbacks"
)

type sendMessageFunc func(message string)

type agentCallbackHandler struct {
	callbacks.SimpleHandler
	sendMessage sendMessageFunc
}

func (handler *agentCallbackHandler) HandleChainEnd(_ context.Context, outputs map[string]any) {
	text, ok := outputs["text"]
	if !ok {
		return
	}
	textStr, ok := text.(string)
	if !ok {
		return
	}

	// Only send the final answer to the user.
	// Intermediate agent steps contain "Action:" and "Action Input:" lines
	// which are internal reasoning and should not be shown.
	// The final answer uses the format:
	//   Thought: Do I need to use a tool? No
	//   AI: <actual response>
	if strings.Contains(textStr, "Action:") && strings.Contains(textStr, "Action Input:") {
		// This is an intermediate tool-calling step — suppress it
		return
	}

	// Strip the ReAct scaffolding prefix from the final answer
	if idx := strings.Index(textStr, "AI:"); idx != -1 {
		textStr = strings.TrimSpace(textStr[idx+3:])
	}

	if textStr != "" {
		handler.sendMessage(textStr)
	}
}
