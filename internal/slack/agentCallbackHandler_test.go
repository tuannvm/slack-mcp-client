package slackbot

import (
	"encoding/json"

	"github.com/slack-go/slack"
	"testing"
)

func TestIsThinkingMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name: "Thinking message",
			input: `
			Do I need to use a tool? Yes
			thinking...
			`,
			expected: true,
		},
		{
			name: "Not thinking message",
			input: `
			Do I need to use a tool? No
			AI: Here is the final response.
			`,
			expected: false,
		},
		// Add more test cases as needed
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isThinkingMessage(tt.input)
			if result != tt.expected {
				t.Errorf("isThinkingMessage() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFormatFinalResponse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "Final response formatting",
			input: `Do I need to use a tool? No
			AI: Here is the final response.`,
			expected: `Here is the final response.`,
		},
		{
			name:     "Fallback final response formatting",
			input:    `This is final response without prefixes.`,
			expected: `This is final response without prefixes.`,
		},
		// Add more test cases as needed
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatFinalResponse(tt.input)
			if result != tt.expected {
				t.Errorf("isThinkingMessage() = \"%s\", want \"%s\"", result, tt.expected)
			}
		})
	}
}

func TestFormatContextMessageBlock(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Simple text context",
			input: "Here is the final response.",
		},
		// Add more test cases as needed
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatContextMessageBlock(tt.input, nil)

			var contextBlock struct {
				Elements slack.ContextElements `json:"elements"`
			}
			if err := json.Unmarshal([]byte(result), &contextBlock); err != nil {
				t.Errorf("Failed to unmarshal block message JSON: %v", err)
			}
		})
	}
}
