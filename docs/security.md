# Security Features

The Slack MCP Client includes comprehensive security features to control access to the bot and its functionality.

## Overview

The security system provides:
- User-based access control
- Channel-based access control  
- Admin user privileges
- Flexible and strict access modes
- Comprehensive logging of access attempts
- Polite rejection messages for unauthorized users

## Configuration

Security is configured through environment variables:

### Basic Settings

- `SECURITY_ENABLED`: Enable/disable security (default: `false`)
- `SECURITY_STRICT_MODE`: Require both user AND channel whitelisting (default: `false`)
- `SECURITY_LOG_UNAUTHORIZED`: Log unauthorized access attempts (default: `true`)

### Access Control Lists

- `SECURITY_ALLOWED_USERS`: Comma-separated list of allowed user IDs
- `SECURITY_ALLOWED_CHANNELS`: Comma-separated list of allowed channel IDs  
- `SECURITY_ADMIN_USERS`: Comma-separated list of admin user IDs

### Customization

- `SECURITY_REJECTION_MESSAGE`: Custom message for unauthorized users

## Access Modes

### Flexible Mode (Default)
When `SECURITY_STRICT_MODE=false`:
- Access is granted if the user OR channel is whitelisted
- Admin users always have access regardless of channel restrictions

### Strict Mode  
When `SECURITY_STRICT_MODE=true`:
- Access requires BOTH user AND channel to be whitelisted
- Admin users still bypass channel restrictions

## Admin Users

Admin users have special privileges:
- Always bypass channel restrictions
- Can interact with the bot in any channel (if user is whitelisted)
- Useful for administrators who need access across all channels

## Example Configuration

```bash
# Enable security with flexible mode
SECURITY_ENABLED=true
SECURITY_STRICT_MODE=false

# Allow specific users
SECURITY_ALLOWED_USERS=U1234567890,U0987654321

# Allow specific channels  
SECURITY_ALLOWED_CHANNELS=C1234567890,C0987654321

# Set admin users
SECURITY_ADMIN_USERS=U1111111111

# Custom rejection message
SECURITY_REJECTION_MESSAGE="Access denied. Please contact your IT administrator."

# Enable logging of unauthorized attempts
SECURITY_LOG_UNAUTHORIZED=true
```

## Getting User and Channel IDs

### User IDs
1. In Slack, right-click on a user's name
2. Select "Copy link"
3. The URL contains the user ID (format: `U1234567890`)

### Channel IDs  
1. In Slack, right-click on a channel name
2. Select "Copy link"
3. The URL contains the channel ID (format: `C1234567890`)

## Logging

When security is enabled, all access attempts are logged with:
- User ID
- Channel ID
- Access decision (granted/denied)
- Reason for decision
- Security mode information

Example log entries:
```
[INFO] security: Access granted user_id=U1234567890 channel_id=C1234567890 allowed=true reason="User whitelisted"
[WARN] security: Access denied user_id=U0000000000 channel_id=C0000000000 allowed=false reason="Neither user nor channel whitelisted"
```

## Security Behavior

### When Security is Disabled
- All users can interact with the bot
- No access control checks are performed
- No security-related logging occurs

### When Security is Enabled
- Only whitelisted users/channels can interact
- Unauthorized users receive a polite rejection message
- All access attempts are logged (if enabled)
- The bot will not respond to unauthorized requests

## Best Practices

1. **Start with Flexible Mode**: Begin with `SECURITY_STRICT_MODE=false` for easier initial setup
2. **Use Admin Users**: Designate admin users for management access
3. **Monitor Logs**: Review security logs regularly for unauthorized access attempts
4. **Custom Messages**: Use professional rejection messages that don't reveal system details
5. **Regular Audits**: Periodically review and update user/channel whitelists

## Troubleshooting

### Bot Not Responding
1. Check if security is enabled: `SECURITY_ENABLED=true`
2. Verify user ID is in `SECURITY_ALLOWED_USERS`
3. In strict mode, verify channel ID is in `SECURITY_ALLOWED_CHANNELS`
4. Check logs for access denial messages

### Getting Access Denied
1. Confirm your user ID is correctly added to the whitelist
2. In strict mode, ensure the channel is also whitelisted
3. Contact an admin user to add you to the appropriate lists

### Admin Access Issues
1. Verify admin user ID is in `SECURITY_ADMIN_USERS`
2. Admin users still need to be in `SECURITY_ALLOWED_USERS` in strict mode
3. Check logs for specific denial reasons
