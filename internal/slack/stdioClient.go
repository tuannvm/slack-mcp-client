package slackbot

import (
	"bufio"
	"fmt"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
	"io"
	"os"
)

type StdioClient struct {
	events chan socketmode.Event
	Output io.Writer
	Input  io.Reader
	logger *logging.Logger
}

func NewStdioClient(stdLogger *logging.Logger) *StdioClient {
	logLevel := getLogLevel(stdLogger)
	stdioLogger := logging.New("stdio-client", logLevel)
	return &StdioClient{
		events: make(chan socketmode.Event, 50),
		Output: os.Stdout,
		Input:  os.Stdin,
		logger: stdioLogger,
	}
}
func (client StdioClient) GetConversationHistory(params *slack.GetConversationHistoryParameters) (*slack.GetConversationHistoryResponse, error) {
	return nil, nil
}
func (client StdioClient) DeleteMessage(channel, messageTimestamp string) (string, string, error) {
	return "", "", nil
}
func (client StdioClient) Run() error {
	scanner := bufio.NewScanner(client.Input)
	for scanner.Scan() {
		e := socketmode.Event{
			Type: socketmode.EventTypeEventsAPI,
			Data: slackevents.EventsAPIEvent{
				Token:        "",
				TeamID:       "",
				Type:         slackevents.CallbackEvent,
				APIAppID:     "",
				EnterpriseID: "",
				Data:         nil,
				InnerEvent: slackevents.EventsAPIInnerEvent{
					Type: "",
					Data: &slackevents.AppMentionEvent{
						User:      "xxx",
						Text:      scanner.Text(),
						TimeStamp: "",
						Channel:   "xxx",
					},
				},
			},
			Request: &socketmode.Request{},
		}
		client.events <- e
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading stdin: %w", err)
	}
	return nil
}
func (client StdioClient) Ack(req socketmode.Request, payload ...interface{}) {

}
func (client StdioClient) GetEventChannel() chan socketmode.Event {
	return client.events
}

func (client StdioClient) RemoveBotMention(msg string) string {
	return msg
}

func (client StdioClient) GetLogger() *logging.Logger {
	return client.logger
}

func (client StdioClient) IsValidUser(userID string) bool {
	return true
}

func (client StdioClient) IsBotUser(userID string) bool {
	return false
}

func (client StdioClient) SendMessage(channelID, threadTS, text string) {
	messages := []string{
		"----- SEND MESSAGE -----\n",
		text, "\n",
		"----- END MESSAGE -----\n",
	}
	for _, msg := range messages {
		_, err := client.Output.Write([]byte(msg))
		if err != nil {
			client.logger.ErrorKV("While writing message to output", "error", err)
		}
	}
}
