package slackbot

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"

	customErrors "github.com/tuannvm/slack-mcp-client/internal/common/errors"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
	"github.com/tuannvm/slack-mcp-client/internal/slack/formatter"
)

type UserFrontend interface {
	Run() error
	Ack(req socketmode.Request, payload ...interface{})
	GetEventChannel() chan socketmode.Event
	RemoveBotMention(msg string) string
	IsValidUser(userID string) bool
	GetLogger() *logging.Logger
	SendMessage(channelID, threadTS, text string)
	GetUserInfo(userID string) (*slack.User, error)
	AddReaction(channelID, timestamp, reaction string) error
	RemoveReaction(channelID, timestamp, reaction string) error
}

func getLogLevel(stdLogger *logging.Logger) logging.LogLevel {
	// Determine log level from environment variable
	logLevel := logging.LevelInfo // Default to INFO
	if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
		logLevel = logging.ParseLevel(envLevel)
		stdLogger.InfoKV("Setting Slack client log level from environment", "level", envLevel)
	}
	return logLevel
}

func GetSlackClient(botToken, appToken string, stdLogger *logging.Logger, thinkingMessage string) (*SlackClient, error) {
	if botToken == "" {
		return nil, fmt.Errorf("SLACK_BOT_TOKEN must be set")
	}
	if appToken == "" {
		return nil, fmt.Errorf("SLACK_APP_TOKEN must be set")
	}
	if !strings.HasPrefix(appToken, "xapp-") {
		return nil, fmt.Errorf("SLACK_APP_TOKEN must have the prefix \"xapp-\"")
	}

	logLevel := getLogLevel(stdLogger)

	// Create a structured logger for the Slack client
	slackLogger := logging.New("slack-client", logLevel)

	// Initialize the API client
	api := slack.New(
		botToken,
		slack.OptionAppLevelToken(appToken),
		// Still using standard logger for Slack API as it expects a standard logger
		slack.OptionLog(slackLogger.StdLogger()),
	)

	// Authenticate with Slack
	authTest, err := api.AuthTestContext(context.Background())
	if err != nil {
		return nil, customErrors.WrapSlackError(err, "authentication_failed", "Failed to authenticate with Slack")
	}

	mentionRegex := regexp.MustCompile(fmt.Sprintf("<@%s>", authTest.UserID))

	// Create the socket mode client
	client := socketmode.New(
		api,
		// Still using standard logger for socket mode as it expects a standard logger
		socketmode.OptionLog(slackLogger.StdLogger()),
		socketmode.OptionDebug(false),
	)

	return &SlackClient{
		Client:          client,
		api:             api,
		botMentionRgx:   mentionRegex,
		botUserID:       authTest.UserID,
		logger:          slackLogger,
		thinkingMessage: thinkingMessage,
	}, nil
}

type SlackClient struct {
	*socketmode.Client
	api             *slack.Client
	botMentionRgx   *regexp.Regexp
	botUserID       string
	logger          *logging.Logger
	thinkingMessage string
}

func (slackClient *SlackClient) GetEventChannel() chan socketmode.Event {
	return slackClient.Events
}

func (slackClient *SlackClient) RemoveBotMention(msg string) string {
	return slackClient.botMentionRgx.ReplaceAllString(msg, "")
}

func (slackClient *SlackClient) GetLogger() *logging.Logger {
	return slackClient.logger
}

func (slackClient *SlackClient) IsValidUser(userID string) bool {
	return userID != "" && !slackClient.IsBotUser(userID)
}

func (slackClient *SlackClient) IsBotUser(userID string) bool {
	return userID == slackClient.botUserID
}

func (slackClient *SlackClient) GetUserInfo(userID string) (*slack.User, error) {
	user, err := slackClient.Client.GetUserInfo(userID)
	if err != nil {
		return nil, fmt.Errorf("while getting user info for %s: %w", userID, err)
	}
	return user, nil
}

// AddReaction adds an emoji reaction to a message
func (slackClient *SlackClient) AddReaction(channelID, timestamp, reaction string) error {
	err := slackClient.AddReactionContext(context.Background(), reaction, slack.NewRefToMessage(channelID, timestamp))
	if err != nil {
		slackClient.logger.ErrorKV("Failed to add reaction", "channel", channelID, "timestamp", timestamp, "reaction", reaction, "error", err)
		return fmt.Errorf("failed to add reaction: %w", err)
	}
	slackClient.logger.DebugKV("Added reaction", "channel", channelID, "timestamp", timestamp, "reaction", reaction)
	return nil
}

// RemoveReaction removes an emoji reaction from a message
func (slackClient *SlackClient) RemoveReaction(channelID, timestamp, reaction string) error {
	err := slackClient.RemoveReactionContext(context.Background(), reaction, slack.NewRefToMessage(channelID, timestamp))
	if err != nil {
		slackClient.logger.ErrorKV("Failed to remove reaction", "channel", channelID, "timestamp", timestamp, "reaction", reaction, "error", err)
		return fmt.Errorf("failed to remove reaction: %w", err)
	}
	slackClient.logger.DebugKV("Removed reaction", "channel", channelID, "timestamp", timestamp, "reaction", reaction)
	return nil
}

// SendMessage sends a message back to Slack, replying in a thread if threadTS is provided.
func (slackClient *SlackClient) SendMessage(channelID, threadTS, text string) {
	if text == "" {
		slackClient.logger.WarnKV("Attempted to send empty message, skipping", "channel", channelID)
		return
	}


	// Detect message type and format accordingly
	messageType := formatter.DetectMessageType(text)
	slackClient.logger.DebugKV("Detected message type", "type", messageType, "length", len(text))

	var msgOptions []slack.MsgOption

	switch messageType {
	case formatter.JSONBlock:
		// Message is already in Block Kit JSON format
		options := formatter.DefaultOptions()
		options.Format = formatter.BlockFormat
		options.ThreadTS = threadTS
		msgOptions = formatter.FormatMessage(text, options)

	case formatter.StructuredData:
		// Convert structured data to Block Kit format
		formattedText := formatter.FormatStructuredData(text)
		options := formatter.DefaultOptions()
		options.Format = formatter.BlockFormat
		options.ThreadTS = threadTS
		msgOptions = formatter.FormatMessage(formattedText, options)

	case formatter.MarkdownText, formatter.PlainText:
		// Check if the text contains markdown images
		if formatter.HasMarkdownImages(text) {
			// Convert to Block Kit format with image blocks
			formattedText := formatter.ConvertMarkdownWithImages(text)
			options := formatter.DefaultOptions()
			options.Format = formatter.BlockFormat
			options.ThreadTS = threadTS
			msgOptions = formatter.FormatMessage(formattedText, options)
		} else {
			// Apply Markdown formatting and use default text formatting
			formattedText := formatter.FormatMarkdown(text)
			options := formatter.DefaultOptions()
			options.ThreadTS = threadTS
			msgOptions = formatter.FormatMessage(formattedText, options)
		}
	}

	// Send the message
	_, _, err := slackClient.PostMessage(channelID, msgOptions...)
	if err != nil {
		slackClient.logger.ErrorKV("Error posting message to channel", "channel", channelID, "error", err, "messageType", messageType)

		// If we get an error with Block Kit format, try falling back to plain text
		if messageType == formatter.JSONBlock || messageType == formatter.StructuredData {
			slackClient.logger.InfoKV("Falling back to plain text format due to Block Kit error", "channel", channelID)

			// Apply markdown formatting to the original text and send as plain text
			formattedText := formatter.FormatMarkdown(text)
			fallbackOptions := []slack.MsgOption{
				slack.MsgOptionText(formattedText, false),
			}
			if threadTS != "" {
				fallbackOptions = append(fallbackOptions, slack.MsgOptionTS(threadTS))
			}

			// Try sending with plain text format
			_, _, fallbackErr := slackClient.PostMessage(channelID, fallbackOptions...)
			if fallbackErr != nil {
				slackClient.logger.ErrorKV("Error posting fallback message to channel", "channel", channelID, "error", fallbackErr)
			}
		}
	}
}
