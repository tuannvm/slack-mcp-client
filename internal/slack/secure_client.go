// Package slackbot provides security-enabled Slack client functionality
package slackbot

import (
	"strings"

	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
	"github.com/tuannvm/slack-mcp-client/internal/config"
	"github.com/tuannvm/slack-mcp-client/internal/mcp"
	"github.com/tuannvm/slack-mcp-client/internal/security"
)

// SecureClient extends the basic Client with security features
type SecureClient struct {
	*Client
	accessController *security.AccessController
}

// NewSecureClient creates a new security-enabled Slack client instance
func NewSecureClient(userFrontend UserFrontend, stdLogger *logging.Logger, mcpClients map[string]*mcp.Client,
	discoveredTools map[string]mcp.ToolInfo, cfg *config.Config) (*SecureClient, error) {

	// Create the base client using the unified config
	baseClient, err := NewClient(userFrontend, stdLogger, mcpClients, discoveredTools, cfg)
	if err != nil {
		return nil, err
	}

	// Create access controller with security configuration
	accessController := security.NewAccessController(cfg.GetSecurityConfig(), baseClient.logger.WithName("security"))

	return &SecureClient{
		Client:           baseClient,
		accessController: accessController,
	}, nil
}

// handleEventMessage processes specific EventsAPI messages with security checks
func (sc *SecureClient) handleEventMessage(event slackevents.EventsAPIEvent) {
	switch event.Type {
	case slackevents.CallbackEvent:
		innerEvent := event.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.AppMentionEvent:
			sc.logger.InfoKV("Received app mention in channel", "channel", ev.Channel, "user", ev.User, "text", ev.Text)

			// Security check for app mentions
			if !sc.checkAccess(ev.User, ev.Channel, ev.TimeStamp) {
				return
			}

			messageText := sc.userFrontend.RemoveBotMention(ev.Text)

			userInfo, err := sc.userFrontend.GetUserInfo(ev.User)
			if err != nil {
				sc.logger.ErrorKV("Failed to get user info", "user", ev.User, "error", err)
				return
			}

			// Use handleUserPrompt for app mentions too, for consistency
			go sc.handleUserPrompt(strings.TrimSpace(messageText), ev.Channel, ev.TimeStamp, userInfo.Profile.DisplayName)

		case *slackevents.MessageEvent:
			isDirectMessage := strings.HasPrefix(ev.Channel, "D")
			isValidUser := sc.userFrontend.IsValidUser(ev.User)
			isNotEdited := ev.SubType != "message_changed"
			isBot := ev.BotID != "" || ev.SubType == "bot_message"

			if isDirectMessage && isValidUser && isNotEdited && !isBot {
				sc.logger.InfoKV("Received direct message in channel", "channel", ev.Channel, "user", ev.User, "text", ev.Text)

				// Security check for direct messages
				if !sc.checkAccess(ev.User, ev.Channel, ev.ThreadTimeStamp) {
					return
				}

				userInfo, err := sc.userFrontend.GetUserInfo(ev.User)
				if err != nil {
					sc.logger.ErrorKV("Failed to get user info", "user", ev.User, "error", err)
					return
				}

				go sc.handleUserPrompt(ev.Text, ev.Channel, ev.ThreadTimeStamp, userInfo.Profile.DisplayName)
			}

		default:
			sc.logger.DebugKV("Unsupported inner event type", "type", ev)
		}
	default:
		sc.logger.DebugKV("Unsupported outer event type", "type", event.Type)
	}
}

// checkAccess performs security access control checks
func (sc *SecureClient) checkAccess(userID, channelID, threadTS string) bool {
	decision := sc.accessController.CheckAccess(userID, channelID)

	if !decision.Allowed {
		// Send polite rejection message
		rejectionMessage := sc.accessController.GetRejectionMessage()
		sc.userFrontend.SendMessage(channelID, threadTS, rejectionMessage)

		sc.logger.WarnKV("Access denied",
			"user_id", userID,
			"channel_id", channelID,
			"reason", decision.Reason,
		)
		return false
	}

	sc.logger.DebugKV("Access granted",
		"user_id", userID,
		"channel_id", channelID,
		"reason", decision.Reason,
	)
	return true
}

// Run starts the Socket Mode event loop with security-enabled event handling
func (sc *SecureClient) Run() error {
	go sc.handleSecureEvents()
	sc.logger.Info("Starting security-enabled Slack Socket Mode listener...")
	return sc.userFrontend.Run()
}

// handleSecureEvents listens for incoming events and dispatches them with security checks
func (sc *SecureClient) handleSecureEvents() {
	for evt := range sc.userFrontend.GetEventChannel() {
		switch evt.Type {
		case socketmode.EventTypeConnecting:
			sc.logger.Info("Connecting to Slack...")
		case socketmode.EventTypeConnectionError:
			sc.logger.Warn("Connection failed. Retrying...")
		case socketmode.EventTypeConnected:
			sc.logger.Info("Connected to Slack!")
		case socketmode.EventTypeEventsAPI:
			eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
			if !ok {
				sc.logger.WarnKV("Ignored unexpected EventsAPI event type", "type", evt.Data)
				continue
			}
			sc.userFrontend.Ack(*evt.Request)
			sc.logger.InfoKV("Received EventsAPI event", "type", eventsAPIEvent.Type)
			sc.handleEventMessage(eventsAPIEvent) // Use security-enabled handler
		default:
			sc.logger.DebugKV("Ignored event type", "type", evt.Type)
		}
	}
	sc.logger.Info("Slack event channel closed.")
}

// UpdateSecurityConfig updates the security configuration at runtime
func (sc *SecureClient) UpdateSecurityConfig(securityConfig *security.SecurityConfig) {
	sc.accessController.UpdateConfig(securityConfig)
}

// GetSecurityConfig returns the current security configuration
func (sc *SecureClient) GetSecurityConfig() security.SecurityConfig {
	return sc.accessController.GetConfig()
}

// IsSecurityEnabled returns whether security is currently enabled
func (sc *SecureClient) IsSecurityEnabled() bool {
	return sc.accessController.IsSecurityEnabled()
}
