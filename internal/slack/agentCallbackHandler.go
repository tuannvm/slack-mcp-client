package slackbot

import (
	"context"
	"encoding/json"
	"strings"
	"github.com/slack-go/slack"
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
			// For now, use plain formatting - Block Kit will be handled by the SendMessage function
			formattedText := formatPlainAgentOutput(textStr)
			handler.sendMessage(formattedText)
		}
	}
}

// formatAgentOutput processes the agent output to create a Block Kit message
func formatAgentOutput(text string) string {
	lines := strings.Split(text, "\n")
	var blocks []slack.Block
	var contextElements []slack.MixedElement
	var currentSection strings.Builder
	inCodeBlock := false
	codeBlockContent := strings.Builder{}
	
	// Helper function to add context block if elements exist
	addContextBlock := func() {
		if len(contextElements) > 0 {
			blocks = append(blocks, slack.NewContextBlock("", contextElements...))
			contextElements = []slack.MixedElement{}
		}
	}
	
	// Helper function to add section block if content exists
	addSectionBlock := func() {
		if currentSection.Len() > 0 {
			blocks = append(blocks, slack.NewSectionBlock(
				slack.NewTextBlockObject("mrkdwn", currentSection.String(), false, false),
				nil, nil,
			))
			currentSection.Reset()
		}
	}
	
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		
		// Check for code blocks
		if strings.HasPrefix(line, "```") {
			if inCodeBlock {
				// End of code block
				addSectionBlock() // Add any pending section
				blocks = append(blocks, slack.NewSectionBlock(
					slack.NewTextBlockObject("mrkdwn", "```\n"+codeBlockContent.String()+"\n```", false, false),
					nil, nil,
				))
				codeBlockContent.Reset()
				inCodeBlock = false
			} else {
				// Start of code block
				addSectionBlock() // Add any pending section
				inCodeBlock = true
			}
			continue
		}
		
		if inCodeBlock {
			if codeBlockContent.Len() > 0 {
				codeBlockContent.WriteString("\n")
			}
			codeBlockContent.WriteString(line)
			continue
		}
		
		// Process thoughts and metadata as context blocks
		if strings.HasPrefix(trimmedLine, "> Thought:") || strings.HasPrefix(trimmedLine, "Thought:") {
			addSectionBlock() // Add any pending section
			thoughtContent := strings.TrimPrefix(trimmedLine, "> ")
			thoughtContent = strings.TrimPrefix(thoughtContent, "Thought:")
			contextElements = append(contextElements, 
				slack.NewTextBlockObject("mrkdwn", "_Thought:_ "+strings.TrimSpace(thoughtContent), false, false))
		} else if strings.HasPrefix(trimmedLine, "Justification:") {
			justContent := strings.TrimPrefix(trimmedLine, "Justification:")
			contextElements = append(contextElements,
				slack.NewTextBlockObject("mrkdwn", "_Justification:_ "+strings.TrimSpace(justContent), false, false))
		} else if strings.HasPrefix(trimmedLine, "Action:") {
			actionContent := strings.TrimPrefix(trimmedLine, "Action:")
			contextElements = append(contextElements,
				slack.NewTextBlockObject("mrkdwn", "_Action:_ `"+strings.TrimSpace(actionContent)+"`", false, false))
		} else if strings.HasPrefix(trimmedLine, "Action Input:") {
			// Action Input often contains JSON, keep it in code formatting
			inputContent := strings.TrimPrefix(trimmedLine, "Action Input:")
			contextElements = append(contextElements,
				slack.NewTextBlockObject("mrkdwn", "_Action Input:_ `"+strings.TrimSpace(inputContent)+"`", false, false))
		} else if strings.HasPrefix(trimmedLine, "Observation:") {
			// Add context blocks before observation
			addContextBlock()
			obsContent := strings.TrimPrefix(trimmedLine, "Observation:")
			contextElements = append(contextElements,
				slack.NewTextBlockObject("mrkdwn", "_Observation:_ "+strings.TrimSpace(obsContent), false, false))
		} else if strings.HasPrefix(trimmedLine, "AI:") {
			// Finish any pending context blocks
			addContextBlock()
			// Remove the "AI: " prefix and treat as regular content
			content := strings.TrimSpace(strings.TrimPrefix(trimmedLine, "AI:"))
			if content != "" {
				// Check if this looks like a header (starts with # or is in all caps)
				if strings.HasPrefix(content, "# ") || strings.HasPrefix(content, "## ") {
					headerText := strings.TrimLeft(content, "# ")
					addSectionBlock() // Add any pending section
					blocks = append(blocks, slack.NewHeaderBlock(
						slack.NewTextBlockObject("plain_text", headerText, false, false),
					))
				} else {
					if currentSection.Len() > 0 {
						currentSection.WriteString("\n")
					}
					currentSection.WriteString(content)
				}
			}
		} else {
			// Regular content
			addContextBlock() // Add any pending context blocks
			
			// Check for markdown headers
			if strings.HasPrefix(trimmedLine, "# ") {
				headerText := strings.TrimPrefix(trimmedLine, "# ")
				addSectionBlock() // Add any pending section
				blocks = append(blocks, slack.NewHeaderBlock(
					slack.NewTextBlockObject("plain_text", headerText, false, false),
				))
			} else if strings.HasPrefix(trimmedLine, "## ") {
				headerText := strings.TrimPrefix(trimmedLine, "## ")
				addSectionBlock() // Add any pending section
				blocks = append(blocks, slack.NewHeaderBlock(
					slack.NewTextBlockObject("plain_text", headerText, false, false),
				))
			} else if strings.HasPrefix(trimmedLine, "### ") {
				// For smaller headers, use bold text in section
				headerText := strings.TrimPrefix(trimmedLine, "### ")
				if currentSection.Len() > 0 {
					currentSection.WriteString("\n")
				}
				currentSection.WriteString("*" + headerText + "*")
			} else {
				// Regular line
				if line != "" || currentSection.Len() > 0 {
					if currentSection.Len() > 0 && line != "" {
						currentSection.WriteString("\n")
					}
					currentSection.WriteString(line)
				}
			}
		}
	}
	
	// Add any remaining content
	addContextBlock()
	addSectionBlock()
	
	// If we have blocks, create a Block Kit message
	if len(blocks) > 0 {
		// Add a divider between thoughts and response if we have context
		hasContext := false
		for _, block := range blocks {
			if _, ok := block.(*slack.ContextBlock); ok {
				hasContext = true
				break
			}
		}
		
		if hasContext {
			// Find where to insert divider (after last context block)
			dividerIndex := -1
			for i, block := range blocks {
				if _, ok := block.(*slack.ContextBlock); ok {
					dividerIndex = i
				}
			}
			
			if dividerIndex >= 0 && dividerIndex < len(blocks)-1 {
				// Insert divider after the last context block
				newBlocks := make([]slack.Block, 0, len(blocks)+1)
				newBlocks = append(newBlocks, blocks[:dividerIndex+1]...)
				newBlocks = append(newBlocks, slack.NewDividerBlock())
				newBlocks = append(newBlocks, blocks[dividerIndex+1:]...)
				blocks = newBlocks
			}
		}
		
		// Convert blocks to JSON format
		blockData := map[string]interface{}{
			"text": text, // Fallback text
			"blocks": blocks,
		}
		
		// Marshal to JSON
		jsonBytes, err := json.Marshal(blockData)
		if err != nil {
			// Fallback to plain formatting
			return formatPlainAgentOutput(text)
		}
		
		return string(jsonBytes)
	}
	
	// Fallback to formatted text if no blocks were created
	return formatPlainAgentOutput(text)
}

// formatPlainAgentOutput formats agent output for better Slack readability
func formatPlainAgentOutput(text string) string {
	lines := strings.Split(text, "\n")
	var result []string
	var metadataLines []string
	inMetadata := false
	
	for i, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		
		// Check if we're starting metadata section
		if strings.HasPrefix(trimmedLine, "Thought:") || strings.HasPrefix(trimmedLine, "> Thought:") ||
		   strings.HasPrefix(trimmedLine, "Justification:") || strings.HasPrefix(trimmedLine, "Action:") ||
		   strings.HasPrefix(trimmedLine, "Action Input:") || strings.HasPrefix(trimmedLine, "Observation:") {
			inMetadata = true
		}
		
		if inMetadata {
			// Format metadata lines with italics, emojis, and proper spacing
			if strings.HasPrefix(trimmedLine, "> Thought:") {
				thoughtContent := strings.TrimPrefix(trimmedLine, "> ")
				thoughtContent = strings.TrimPrefix(thoughtContent, "Thought:")
				metadataLines = append(metadataLines, ":brain: _Thought:_ "+strings.TrimSpace(thoughtContent))
			} else if strings.HasPrefix(trimmedLine, "Thought:") {
				thoughtContent := strings.TrimPrefix(trimmedLine, "Thought:")
				metadataLines = append(metadataLines, ":brain: _Thought:_ "+strings.TrimSpace(thoughtContent))
			} else if strings.HasPrefix(trimmedLine, "Justification:") {
				justContent := strings.TrimPrefix(trimmedLine, "Justification:")
				metadataLines = append(metadataLines, ":scales: _Justification:_ "+strings.TrimSpace(justContent))
			} else if strings.HasPrefix(trimmedLine, "Action:") {
				actionContent := strings.TrimPrefix(trimmedLine, "Action:")
				metadataLines = append(metadataLines, ":right-facing_fist: _Action:_ "+strings.TrimSpace(actionContent))
			} else if strings.HasPrefix(trimmedLine, "Action Input:") {
				// Keep JSON in code blocks
				metadataLines = append(metadataLines, ":arrow_right: _Action Input:_")
				// Look for the JSON on the same line or next lines
				jsonContent := strings.TrimPrefix(trimmedLine, "Action Input:")
				jsonContent = strings.TrimSpace(jsonContent)
				if jsonContent != "" {
					metadataLines = append(metadataLines, "```")
					metadataLines = append(metadataLines, jsonContent)
					metadataLines = append(metadataLines, "```")
				} else if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" {
					// JSON might be on next line
					metadataLines = append(metadataLines, "```")
				}
			} else if strings.HasPrefix(trimmedLine, "Observation:") {
				obsContent := strings.TrimPrefix(trimmedLine, "Observation:")
				metadataLines = append(metadataLines, ":mag: _Observation:_ "+strings.TrimSpace(obsContent))
			} else if strings.HasPrefix(trimmedLine, "AI:") {
				// End of metadata, start of response
				inMetadata = false
				// Add metadata with separator
				if len(metadataLines) > 0 {
					result = append(result, metadataLines...)
					result = append(result, "───────────") // Visual separator
					result = append(result, "") // Blank line
				}
				// Process AI response
				content := strings.TrimSpace(strings.TrimPrefix(trimmedLine, "AI:"))
				if content != "" {
					// Convert markdown headers to bold
					if strings.HasPrefix(content, "## ") {
						content = "*" + strings.TrimPrefix(content, "## ") + "*"
					} else if strings.HasPrefix(content, "# ") {
						content = "*" + strings.TrimPrefix(content, "# ") + "*"
					}
					result = append(result, content)
				}
			} else if trimmedLine == "```" && len(metadataLines) > 0 && metadataLines[len(metadataLines)-1] == "```" {
				// Skip duplicate ``` markers
				continue
			} else if trimmedLine != "" {
				// Part of metadata content
				metadataLines = append(metadataLines, line)
			}
		} else {
			// Regular content processing
			if strings.HasPrefix(trimmedLine, "## ") {
				// Convert ## headers to bold
				result = append(result, "*"+strings.TrimPrefix(trimmedLine, "## ")+"*")
			} else if strings.HasPrefix(trimmedLine, "# ") {
				// Convert # headers to bold
				result = append(result, "*"+strings.TrimPrefix(trimmedLine, "# ")+"*")
			} else if strings.HasPrefix(trimmedLine, "### ") {
				// Convert ### headers to bold
				result = append(result, "*"+strings.TrimPrefix(trimmedLine, "### ")+"*")
			} else {
				// Keep line as-is
				if line != "" || len(result) > 0 {
					result = append(result, line)
				}
			}
		}
	}
	
	// If we ended while still in metadata (no AI: response), add the metadata
	if inMetadata && len(metadataLines) > 0 {
		result = append(result, metadataLines...)
	}
	
	return strings.Join(result, "\n")
}
