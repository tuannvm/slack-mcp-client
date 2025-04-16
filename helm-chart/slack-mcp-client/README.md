# Slack MCP Client Helm Chart

This Helm chart deploys the [slack-mcp-client](https://github.com/tuannvm/slack-mcp-client) to Kubernetes.

## Prerequisites

- Kubernetes 1.16+
- Helm 3.0+
- Slack Bot and App tokens

## Installing the Chart

To install the chart with the release name `my-slack-bot`:

```bash
# Create a values file with your configuration
cat > values.yaml << EOL
secret:
  create: true

env:
  SLACK_BOT_TOKEN: "xoxb-your-bot-token"
  SLACK_APP_TOKEN: "xapp-your-app-token"
  OPENAI_API_KEY: "sk-your-openai-key"
  OPENAI_MODEL: "gpt-4o"
  MCP_MODE: "sse"
  LOG_LEVEL: "info"

# Optional: Configure MCP servers
configMap:
  create: true
EOL

# Install the chart
helm install my-slack-bot ./helm-chart/slack-mcp-client -f values.yaml
```

## Configuration

The following table lists the configurable parameters for the slack-mcp-client chart.

| Parameter          | Description                                         | Default                            |
| ------------------ | --------------------------------------------------- | ---------------------------------- |
| `replicaCount`     | Number of pod replicas                              | `1`                                |
| `image.repository` | Image repository                                    | `ghcr.io/tuannvm/slack-mcp-client` |
| `image.pullPolicy` | Image pull policy                                   | `IfNotPresent`                     |
| `image.tag`        | Image tag                                           | `latest`                           |
| `env.OPENAI_MODEL` | OpenAI model to use                                 | `gpt-4o`                           |
| `env.MCP_MODE`     | MCP transport mode                                  | `sse`                              |
| `env.LOG_LEVEL`    | Logging level                                       | `info`                             |
| `secret.create`    | Whether to create a secret for sensitive data       | `false`                            |
| `secret.name`      | Name of existing secret to use                      | `""`                               |
| `configMap.create` | Whether to create a configmap for MCP server config | `false`                            |
| `configMap.name`   | Name of existing configmap to use                   | `""`                               |
| `configMap.data`   | Additional data to add to the configmap             | `{}`                               |

## Using External Secrets

For production deployments, it's recommended to manage sensitive data using a solution like [External Secrets Operator](https://external-secrets.io/):

```yaml
secret:
  create: false
  name: "slack-mcp-client-secrets"  # Name of your externally managed secret

# Then ensure your secret contains:
# - slack-bot-token
# - slack-app-token
# - openai-api-key
``` 