package slackbot

import (
	"context"
	"github.com/tmc/langchaingo/callbacks"
)

type sendMessageFunc func(message string)

type agentCallbackHandler struct {
	callbacks.SimpleHandler
	sendMessage sendMessageFunc
}

func (handler *agentCallbackHandler) HandleChainEnd(_ context.Context, outputs map[string]any) {
	if text, ok := outputs["text"]; ok {
		if textStr, ok := text.(string); ok {
			handler.sendMessage(textStr)
		}
	}
}
