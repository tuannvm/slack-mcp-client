package mcp

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewSSEMCPClientWithRetry_HeadersPropagation(t *testing.T) {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer some-token")
	headers.Set("Custom-Header", "custom-value")

	client, err := NewSSEMCPClientWithRetry("http://example.com", headers, nil)
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, "Bearer some-token", client.headers.Get("Authorization"))
	assert.Equal(t, "custom-value", client.headers.Get("Custom-Header"))
}
