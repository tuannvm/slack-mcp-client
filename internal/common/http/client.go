// Package http provides an HTTP client with retries, timeouts, and logging capabilities
package http

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"
)

// ClientOptions configures the HTTP client behavior
type ClientOptions struct {
	Timeout        time.Duration
	MaxRetries     int
	RetryBackoff   time.Duration
	MaxBackoff     time.Duration
	RequestLogger  func(method, url string, body []byte)
	ResponseLogger func(statusCode int, body []byte, err error)
}

// DefaultOptions returns sensible default client options
func DefaultOptions() ClientOptions {
	return ClientOptions{
		Timeout:        30 * time.Second,
		MaxRetries:     3,
		RetryBackoff:   500 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		RequestLogger:  func(_, _ string, _ []byte) {},    // nolint:revive // Using underscores for unused parameters
		ResponseLogger: func(_ int, _ []byte, _ error) {}, // nolint:revive // Using underscores for unused parameters
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

	// Marshal request body if present
	if body != nil {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	c.options.RequestLogger(method, url, bodyBytes)

	// Perform the request with retries
	return c.performRequestWithRetries(ctx, method, url, bodyBytes, headers)
}

// performRequestWithRetries handles retrying HTTP requests with exponential backoff
func (c *Client) performRequestWithRetries(ctx context.Context, method, url string, bodyBytes []byte, headers map[string]string) ([]byte, int, error) {
	var statusCode int
	var responseBytes []byte
	var err error

	backoff := c.options.RetryBackoff

	for attempt := 0; attempt <= c.options.MaxRetries; attempt++ {
		// Apply backoff delay after the first attempt
		if err = c.applyBackoffDelay(ctx, attempt, &backoff); err != nil {
			return nil, 0, err
		}

		// Create and execute the request
		responseBytes, statusCode, err = c.executeRequest(ctx, method, url, bodyBytes, headers)

		// Check if we should break the retry loop
		shouldRetry := c.shouldRetryRequest(statusCode, err)
		if !shouldRetry {
			break
		}
	}

	c.options.ResponseLogger(statusCode, responseBytes, err)

	// Check for non-success status code
	if statusCode < 200 || statusCode >= 300 {
		return responseBytes, statusCode, fmt.Errorf("request failed with status %d: %s", statusCode, string(responseBytes))
	}

	return responseBytes, statusCode, err
}

// applyBackoffDelay waits an appropriate time between retry attempts
func (c *Client) applyBackoffDelay(ctx context.Context, attempt int, backoff *time.Duration) error {
	if attempt <= 0 {
		return nil // No delay needed for the first attempt
	}

	// Add jitter to backoff to prevent thundering herd
	// Use crypto/rand instead of math/rand for secure random number generation
	maxJitter := int64(*backoff) / 2
	randomBig, err := rand.Int(rand.Reader, big.NewInt(maxJitter))
	if err != nil {
		return fmt.Errorf("failed to generate secure random number: %w", err)
	}
	jitter := time.Duration(randomBig.Int64())
	sleepTime := *backoff + jitter

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(sleepTime):
		// Continue with retry
	}

	// Exponential backoff
	*backoff *= 2
	if *backoff > c.options.MaxBackoff {
		*backoff = c.options.MaxBackoff
	}

	return nil
}

// executeRequest performs a single HTTP request
func (c *Client) executeRequest(ctx context.Context, method, url string, bodyBytes []byte, headers map[string]string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, 0, err
	}

	// Set request headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	if len(bodyBytes) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	// Execute the request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, err
	}

	statusCode := resp.StatusCode

	// Read response body
	responseBytes, err := io.ReadAll(resp.Body)

	// Close response body and check for errors
	closeErr := resp.Body.Close()
	if closeErr != nil {
		c.options.ResponseLogger(statusCode, nil, closeErr)
		return nil, statusCode, fmt.Errorf("failed to close response body: %w", closeErr)
	}

	return responseBytes, statusCode, err
}

// shouldRetryRequest determines if a request should be retried based on status code and error
func (c *Client) shouldRetryRequest(statusCode int, err error) bool {
	if err != nil {
		return true // Retry on any error
	}

	// Don't retry on 4xx client errors (except 429 - too many requests)
	if statusCode >= 400 && statusCode < 500 && statusCode != 429 {
		return false
	}

	// No need to retry on success
	if statusCode >= 200 && statusCode < 300 {
		return false
	}

	// Retry on other status codes (e.g., 5xx)
	return true
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
