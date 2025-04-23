// Package errors provides a unified error handling system for the application
package errors

import (
	"errors"
	"fmt"
	"runtime"
)

// ErrorDomain represents the domain/component where an error originated
type ErrorDomain string

// Define error domains for different components of the application
const (
	// ErrorDomainConfig represents errors from the configuration system
	ErrorDomainConfig ErrorDomain = "config"

	// ErrorDomainLLM represents errors from language model interactions
	ErrorDomainLLM ErrorDomain = "llm"

	// ErrorDomainSlack represents errors from Slack API interactions
	ErrorDomainSlack ErrorDomain = "slack"

	// ErrorDomainMCP represents errors from MCP protocol
	ErrorDomainMCP ErrorDomain = "mcp"

	// ErrorDomainHTTP represents errors from HTTP operations
	ErrorDomainHTTP ErrorDomain = "http"

	// ErrorDomainInternal represents internal application errors
	ErrorDomainInternal ErrorDomain = "internal"
)

// DomainError is the central error type for the application,
// providing structured information about the error context
type DomainError struct {
	// Domain is the component where the error originated
	Domain ErrorDomain

	// Code is a machine-readable identifier for the error type
	Code string

	// Message is a human-readable description of the error
	Message string

	// Cause is the underlying error that led to this error
	Cause error

	// Stack contains the stack trace at the point of error creation
	Stack string

	// Data contains additional contextual data about the error
	Data map[string]interface{}
}

// Error implements the error interface
func (e *DomainError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s:%s] %s: %v", e.Domain, e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s:%s] %s", e.Domain, e.Code, e.Message)
}

// Unwrap returns the underlying cause, implementing the unwrap interface
func (e *DomainError) Unwrap() error {
	return e.Cause
}

// WithData adds contextual data to the error
func (e *DomainError) WithData(key string, value interface{}) *DomainError {
	if e.Data == nil {
		e.Data = make(map[string]interface{})
	}
	e.Data[key] = value
	return e
}

// NewDomainError creates a new DomainError with the given domain, code, and message
func NewDomainError(domain ErrorDomain, code, message string) *DomainError {
	return &DomainError{
		Domain:  domain,
		Code:    code,
		Message: message,
		Stack:   captureStack(2), // Skip this function and caller
	}
}

// WrapWithDomain creates a new DomainError that wraps an existing error
func WrapWithDomain(err error, domain ErrorDomain, code, message string) *DomainError {
	if err == nil {
		return nil
	}
	return &DomainError{
		Domain:  domain,
		Code:    code,
		Message: message,
		Cause:   err,
		Stack:   captureStack(2), // Skip this function and caller
	}
}

// NewDomainErrorf creates a new DomainError with formatted message
func NewDomainErrorf(domain ErrorDomain, code string, format string, args ...interface{}) *DomainError {
	return NewDomainError(domain, code, fmt.Sprintf(format, args...))
}

// WrapWithDomainf creates a new DomainError with formatted message, wrapping an existing error
func WrapWithDomainf(err error, domain ErrorDomain, code string, format string, args ...interface{}) *DomainError {
	if err == nil {
		return nil
	}
	return WrapWithDomain(err, domain, code, fmt.Sprintf(format, args...))
}

// IsDomainError checks if an error is a DomainError
func IsDomainError(err error) bool {
	var domainErr *DomainError
	return errors.As(err, &domainErr)
}

// GetDomain extracts the domain from an error if it's a DomainError
func GetDomain(err error) (ErrorDomain, bool) {
	var domainErr *DomainError
	if errors.As(err, &domainErr) {
		return domainErr.Domain, true
	}
	return "", false
}

// GetErrorCode extracts the code from an error if it's a DomainError
func GetErrorCode(err error) (string, bool) {
	var domainErr *DomainError
	if errors.As(err, &domainErr) {
		return domainErr.Code, true
	}
	return "", false
}

// GetErrorData extracts data from an error if it's a DomainError
func GetErrorData(err error, key string) (interface{}, bool) {
	var domainErr *DomainError
	if errors.As(err, &domainErr) && domainErr.Data != nil {
		val, ok := domainErr.Data[key]
		return val, ok
	}
	return nil, false
}

// captureStack captures the current goroutine's stack trace
func captureStack(skip int) string {
	const depth = 32
	var pcs [depth]uintptr
	n := runtime.Callers(skip+1, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])

	var stackBuilder string
	for {
		frame, more := frames.Next()
		if !more {
			break
		}
		stackBuilder += fmt.Sprintf("%s\n\t%s:%d\n", frame.Function, frame.File, frame.Line)
		if !more {
			break
		}
	}
	return stackBuilder
}

// Convenience functions for creating domain-specific errors

// NewConfigError creates a new error in the config domain
func NewConfigError(code, message string) *DomainError {
	return NewDomainError(ErrorDomainConfig, code, message)
}

// NewConfigErrorf creates a new formatted error in the config domain
func NewConfigErrorf(code string, format string, args ...interface{}) *DomainError {
	return NewDomainErrorf(ErrorDomainConfig, code, format, args...)
}

// WrapConfigError wraps an error in the config domain
func WrapConfigError(err error, code, message string) *DomainError {
	return WrapWithDomain(err, ErrorDomainConfig, code, message)
}

// NewLLMError creates a new error in the LLM domain
func NewLLMError(code, message string) *DomainError {
	return NewDomainError(ErrorDomainLLM, code, message)
}

// NewLLMErrorf creates a new formatted error in the LLM domain
func NewLLMErrorf(code string, format string, args ...interface{}) *DomainError {
	return NewDomainErrorf(ErrorDomainLLM, code, format, args...)
}

// WrapLLMError wraps an error in the LLM domain
func WrapLLMError(err error, code, message string) *DomainError {
	return WrapWithDomain(err, ErrorDomainLLM, code, message)
}

// NewSlackError creates a new error in the Slack domain
func NewSlackError(code, message string) *DomainError {
	return NewDomainError(ErrorDomainSlack, code, message)
}

// NewSlackErrorf creates a new formatted error in the Slack domain
func NewSlackErrorf(code string, format string, args ...interface{}) *DomainError {
	return NewDomainErrorf(ErrorDomainSlack, code, format, args...)
}

// WrapSlackError wraps an error in the Slack domain
func WrapSlackError(err error, code, message string) *DomainError {
	return WrapWithDomain(err, ErrorDomainSlack, code, message)
}

// NewMCPError creates a new error in the MCP domain
func NewMCPError(code, message string) *DomainError {
	return NewDomainError(ErrorDomainMCP, code, message)
}

// NewMCPErrorf creates a new formatted error in the MCP domain
func NewMCPErrorf(code string, format string, args ...interface{}) *DomainError {
	return NewDomainErrorf(ErrorDomainMCP, code, format, args...)
}

// WrapMCPError wraps an error in the MCP domain
func WrapMCPError(err error, code, message string) *DomainError {
	return WrapWithDomain(err, ErrorDomainMCP, code, message)
}

// NewHTTPError creates a new error in the HTTP domain
func NewHTTPError(code, message string) *DomainError {
	return NewDomainError(ErrorDomainHTTP, code, message)
}

// NewHTTPErrorf creates a new formatted error in the HTTP domain
func NewHTTPErrorf(code string, format string, args ...interface{}) *DomainError {
	return NewDomainErrorf(ErrorDomainHTTP, code, format, args...)
}

// WrapHTTPError wraps an error in the HTTP domain
func WrapHTTPError(err error, code, message string) *DomainError {
	return WrapWithDomain(err, ErrorDomainHTTP, code, message)
}

// NewInternalError creates a new error in the internal domain
func NewInternalError(code, message string) *DomainError {
	return NewDomainError(ErrorDomainInternal, code, message)
}

// NewInternalErrorf creates a new formatted error in the internal domain
func NewInternalErrorf(code string, format string, args ...interface{}) *DomainError {
	return NewDomainErrorf(ErrorDomainInternal, code, format, args...)
}

// WrapInternalError wraps an error in the internal domain
func WrapInternalError(err error, code, message string) *DomainError {
	return WrapWithDomain(err, ErrorDomainInternal, code, message)
}
