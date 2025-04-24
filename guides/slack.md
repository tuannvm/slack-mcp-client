# Slack Configuration Guide

This document provides instructions for setting up your Slack app for use with the `slack-mcp-client` application.

## Slack App Setup Requirements

### Token Types

The application requires two types of tokens:

1. **Bot Token** (`SLACK_BOT_TOKEN`): Starts with `xoxb-`
2. **App-Level Token** (`SLACK_APP_TOKEN`): Starts with `xapp-`

### Required Bot Token Scopes

Add these OAuth scopes to your Bot Token in the "OAuth & Permissions" section:

**Essential Scopes:**
- `app_mentions:read` - Allows the bot to receive mention events
- `chat:write` - Allows the bot to post messages
- `im:history` - Allows the bot to read direct messages
- `im:read` - Allows the bot to receive DM events

**Additional Scopes (for full functionality):**
- `channels:history` - Allows reading public channel history (needed for message management)
- `groups:history` - Allows reading private channel history
- `mpim:history` - Allows reading multi-person IM history

### App-Level Token Configuration

1. Go to the "Socket Mode" section in your Slack app settings
2. Enable Socket Mode
3. Create an App-Level Token with the `connections:write` scope
4. Use this token for the `SLACK_APP_TOKEN` environment variable

### Event Subscriptions

In the "Event Subscriptions" section:

1. Turn ON Event Subscriptions
2. Under "Subscribe to bot events", add these event subscriptions:
   - `message.im` - For direct messages to your app
   - `app_mention` - For mentions of your app in channels

### App Home Configuration

In the "App Home" section:

1. Enable the Messages Tab
2. Turn ON "Allow users to send Slash commands and messages from the messages tab"

## Troubleshooting Common Issues

### "Sending messages to this app has been turned off"

If you see this error:

1. Check the App Home settings as described above
2. Verify Event Subscriptions are properly configured
3. Ensure the app is properly installed to your workspace with the required scopes
4. Check if a workspace admin has restricted DMs with apps
5. Try reinstalling the app to reset permissions

### Message Delivery Issues

If the bot isn't receiving or responding to messages:

1. Confirm Socket Mode is enabled
2. Verify both tokens are valid and properly set in your environment
3. Check that your application is running and connected to Slack
4. Look for any errors in your application logs
5. Ensure your network allows connections to Slack's API servers

## Environment Variables

Configure these environment variables in your `.env` file:

```
SLACK_BOT_TOKEN="xoxb-your-bot-token"
SLACK_APP_TOKEN="xapp-your-app-token"
```

## Installation Process

1. Create a new Slack app at https://api.slack.com/apps
2. Configure all the settings described above
3. Install the app to your workspace
4. Copy the Bot Token and App-Level Token to your environment variables
5. Start the application

## Runtime Verification

When the application starts successfully, it should:

1. Log "Connected to Slack!" in your application logs
2. The bot should appear online in your Slack workspace
3. You should be able to send direct messages to the bot and mention it in channels 