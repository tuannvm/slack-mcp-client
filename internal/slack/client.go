package slack

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"github.com/tuannvm/slack-mcp-client/internal/config"
	"github.com/tuannvm/slack-mcp-client/internal/mcp"
)

// Client manages the connection and interaction with Slack.
type Client struct {
	API       *slack.Client
	Socket    *socketmode.Client
	log       *log.Logger
	mcpClient *mcp.Client
}

// NewClient creates and initializes a new Slack client using Socket Mode.
func NewClient(cfg *config.Config, logger *log.Logger, mcpClient *mcp.Client) (*Client, error) {
	api := slack.New(
		cfg.SlackBotToken,
		slack.OptionAppLevelToken(cfg.SlackAppToken),
		slack.OptionLog(log.New(logger.Writer(), "slack-api: ", log.Lshortfile|log.LstdFlags)),
	)

	socketClient := socketmode.New(
		api,
		socketmode.OptionLog(log.New(logger.Writer(), "slack-socket: ", log.Lshortfile|log.LstdFlags)),
	)

	return &Client{
		API:       api,
		Socket:    socketClient,
		log:       logger,
		mcpClient: mcpClient,
	}, nil
}

// Run starts the Socket Mode event loop and event handling.
// This function will block until the connection closes or an error occurs.
func (c *Client) Run() error {
	// Start a goroutine to handle incoming events from Slack.
	go c.handleEvents()

	c.log.Println("Starting Slack Socket Mode listener...")
	// Run() blocks until the connection is closed.
	return c.Socket.Run()
}

// handleEvents listens for incoming events and dispatches them.
func (c *Client) handleEvents() {
	for evt := range c.Socket.Events {
		switch evt.Type {
		case socketmode.EventTypeConnecting:
			c.log.Println("Connecting to Slack...")
		case socketmode.EventTypeConnectionError:
			c.log.Println("Connection failed. Retrying...")
		case socketmode.EventTypeConnected:
			c.log.Println("Connected to Slack!")
		case socketmode.EventTypeEventsAPI:
			eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
			if !ok {
				c.log.Printf("Ignored unexpected EventsAPI event type: %T", evt.Data)
				continue
			}
			c.log.Printf("Received EventsAPI event: Type=%s", eventsAPIEvent.Type)
			c.Socket.Ack(*evt.Request) // Acknowledge the event
			c.handleEventMessage(eventsAPIEvent)

		// TODO: Handle other event types like interactive components, slash commands etc.
		default:
			c.log.Printf("Ignored event type: %s", evt.Type)
		}
	}
	// This point is reached when the Events channel is closed.
	c.log.Println("Slack event channel closed.")
}

// handleEventMessage processes specific EventsAPI messages.
func (c *Client) handleEventMessage(event slackevents.EventsAPIEvent) {
	switch event.Type {
	case slackevents.CallbackEvent:
		innerEvent := event.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.AppMentionEvent:
			c.log.Printf("Received app mention: User=%s, Channel=%s, Text='%s'", ev.User, ev.Channel, ev.Text)

			// Call the hello tool via MCP client
			// Use the user ID as the name for simplicity for now
			// Add a timeout to the context for the MCP call
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// Extract user name or ID (simple extraction for now)
			// We could fetch user info using c.API.GetUserInfo(ev.User) for a real name
			userName := fmt.Sprintf("<@%s>", ev.User) // Mention the user back

			message, err := c.mcpClient.CallHelloTool(ctx, userName)
			if err != nil {
				c.log.Printf("Failed to call MCP hello tool: %v", err)
				// Optionally notify the user about the error
				_, _, postErr := c.API.PostMessage(ev.Channel, slack.MsgOptionText(fmt.Sprintf("Sorry <@%s>, I encountered an error: %v", ev.User, err), false))
				if postErr != nil {
					c.log.Printf("Failed to post error message to Slack: %v", postErr)
				}
				return
			}

			// Post the response from the tool back to the channel
			_, _, err = c.API.PostMessage(ev.Channel, slack.MsgOptionText(message, false))
			if err != nil {
				c.log.Printf("Failed to post message to Slack: %v", err)
			}

		case *slackevents.MessageEvent: // Handles direct messages and channel messages (if subscribed)
			// Ignore messages from bots or message edits/deletions for now
			if ev.BotID != "" || ev.SubType == "message_changed" || ev.SubType == "message_deleted" {
				return
			}
			c.log.Printf("Received message: User=%s, Channel=%s, Text='%s'", ev.User, ev.Channel, ev.Text)
			// Process direct messages (DMs)
			// Check if this is a DM channel (starts with 'D')
			if len(ev.Channel) > 0 && ev.Channel[0] == 'D' {
				// Add a timeout to the context for the MCP call
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				// Use the user's ID or extract name from the message
				userName := fmt.Sprintf("<@%s>", ev.User) // Mention the user back

				// Call the hello tool via MCP client
				message, err := c.mcpClient.CallHelloTool(ctx, userName)
				if err != nil {
					c.log.Printf("Failed to call MCP hello tool for DM: %v", err)
					// Notify the user about the error
					_, _, postErr := c.API.PostMessage(ev.Channel, slack.MsgOptionText(fmt.Sprintf("Sorry, I encountered an error: %v", err), false))
					if postErr != nil {
						c.log.Printf("Failed to post error message to Slack: %v", postErr)
					}
					return
				}

				// Post the response from the tool back to the DM channel
				_, _, err = c.API.PostMessage(ev.Channel, slack.MsgOptionText(message, false))
				if err != nil {
					c.log.Printf("Failed to post message to Slack DM: %v", err)
				}
			}
		default:
			c.log.Printf("Ignored callback event type: %T", innerEvent.Data)
		}
	default:
		c.log.Printf("Ignored outer event type: %s", event.Type)
	}
}
