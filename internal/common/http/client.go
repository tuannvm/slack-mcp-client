package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"
)

// ClientOptions configures the HTTP client behavior
type ClientOptions struct {
	Timeout       time.Duration
	MaxRetries    int
	RetryBackoff  time.Duration
	MaxBackoff    time.Duration
	RequestLogger func(method, url string, body []byte)
	ResponseLogger func(statusCode int, body []byte, err error)
}

// DefaultOptions returns sensible default client options
func DefaultOptions() ClientOptions {
	return ClientOptions{
		Timeout:      30 * time.Second,
		MaxRetries:   3,
		RetryBackoff: 500 * time.Millisecond,
		MaxBackoff:   5 * time.Second,
		RequestLogger: func(method, url string, body []byte) {},
		ResponseLogger: func(statusCode int, body []byte, err error) {},
	}
}

// Client is a wrapper around http.Client with additional functionality
type Client struct {
	client  *http.Client
	options ClientOptions
}

// NewClient creates a new HTTP client with the given options
func NewClient(options ClientOptions) *Client {
	return &Client{
		client: &http.Client{
			Timeout: options.Timeout,
		},
		options: options,
	}
}

// DoRequest performs an HTTP request with retries and logging
func (c *Client) DoRequest(ctx context.Context, method, url string, body interface{}, headers map[string]string) ([]byte, int, error) {
	var bodyBytes []byte
	var err error

	if body != nil {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	c.options.RequestLogger(method, url, bodyBytes)

	var statusCode int
	var responseBytes []byte
	
	backoff := c.options.RetryBackoff
	
	for attempt := 0; attempt <= c.options.MaxRetries; attempt++ {
		if attempt > 0 {
			// Add jitter to backoff to prevent thundering herd
			jitter := time.Duration(rand.Int63n(int64(backoff) / 2))
			sleepTime := backoff + jitter
			
			select {
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			case <-time.After(sleepTime):
				// Continue with retry
			}
			
			// Exponential backoff
			backoff *= 2
			if backoff > c.options.MaxBackoff {
				backoff = c.options.MaxBackoff
			}
		}
		
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(bodyBytes))
		if err != nil {
			continue
		}

		for key, value := range headers {
			req.Header.Set(key, value)
		}
		
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.client.Do(req)
		if err != nil {
			continue
		}
		
		statusCode = resp.StatusCode
		
		responseBytes, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		
		if err != nil {
			continue
		}
		
		// Don't retry on 4xx client errors (except 429)
		if statusCode >= 400 && statusCode < 500 && statusCode != 429 {
			break
		}
		
		// Success!
		if statusCode >= 200 && statusCode < 300 {
			break
		}
	}
	
	c.options.ResponseLogger(statusCode, responseBytes, err)
	
	if statusCode < 200 || statusCode >= 300 {
		return responseBytes, statusCode, fmt.Errorf("request failed with status %d: %s", statusCode, string(responseBytes))
	}
	
	return responseBytes, statusCode, err
}

// DoJSONRequest performs an HTTP request and unmarshals the response into the target
func (c *Client) DoJSONRequest(ctx context.Context, method, url string, requestBody, responseTarget interface{}, headers map[string]string) (int, error) {
	respBody, statusCode, err := c.DoRequest(ctx, method, url, requestBody, headers)
	if err != nil {
		return statusCode, err
	}
	
	if responseTarget != nil {
		if err := json.Unmarshal(respBody, responseTarget); err != nil {
			return statusCode, fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}
	
	return statusCode, nil
} 