package errors

import (
	"errors"
	"fmt"
)

// Standard errors that can be compared directly
var (
	// ErrNotFound indicates that a resource was not found
	ErrNotFound = errors.New("resource not found")
	
	// ErrUnauthorized indicates that the request lacks valid authentication
	ErrUnauthorized = errors.New("unauthorized request")
	
	// ErrForbidden indicates that the request is not allowed
	ErrForbidden = errors.New("access forbidden")
	
	// ErrBadRequest indicates that the request was invalid
	ErrBadRequest = errors.New("bad request")

	// ErrTimeout indicates that the operation timed out
	ErrTimeout = errors.New("operation timed out")
	
	// ErrTooManyRequests indicates rate limiting
	ErrTooManyRequests = errors.New("too many requests")
	
	// ErrInternal indicates an internal server error
	ErrInternal = errors.New("internal server error")
	
	// ErrUnavailable indicates that the service is currently unavailable
	ErrUnavailable = errors.New("service unavailable")
)

// StatusCodeToError maps HTTP status codes to appropriate errors
func StatusCodeToError(statusCode int) error {
	switch statusCode {
	case 400:
		return ErrBadRequest
	case 401:
		return ErrUnauthorized
	case 403:
		return ErrForbidden
	case 404:
		return ErrNotFound
	case 408:
		return ErrTimeout
	case 429:
		return ErrTooManyRequests
	case 500:
		return ErrInternal
	case 503:
		return ErrUnavailable
	default:
		if statusCode >= 400 && statusCode < 500 {
			return fmt.Errorf("client error: status code %d", statusCode)
		}
		if statusCode >= 500 {
			return fmt.Errorf("server error: status code %d", statusCode)
		}
		return nil
	}
}

// ServiceError represents an error from a specific service
type ServiceError struct {
	Service  string
	Message  string
	Code     string
	Retryable bool
	Cause    error
}

// Error implements the error interface
func (e *ServiceError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s service error: %s (code: %s): %v", 
			e.Service, e.Message, e.Code, e.Cause)
	}
	return fmt.Sprintf("%s service error: %s (code: %s)", 
		e.Service, e.Message, e.Code)
}

// Unwrap implements the errors.Unwrap interface
func (e *ServiceError) Unwrap() error {
	return e.Cause
}

// Is checks if the target error matches this error
func (e *ServiceError) Is(target error) bool {
	t, ok := target.(*ServiceError)
	if !ok {
		return errors.Is(e.Cause, target)
	}
	
	return (t.Service == "" || t.Service == e.Service) &&
		(t.Code == "" || t.Code == e.Code)
}

// NewOpenAIError creates a new error specific to OpenAI
func NewOpenAIError(message string, code string, cause error) error {
	return &ServiceError{
		Service:   "openai",
		Message:   message,
		Code:      code,
		Retryable: code == "rate_limit_exceeded" || code == "server_error",
		Cause:     cause,
	}
}

// NewOllamaError creates a new error specific to Ollama
func NewOllamaError(message string, code string, cause error) error {
	return &ServiceError{
		Service:   "ollama", 
		Message:   message,
		Code:      code,
		Retryable: code == "server_error" || code == "unavailable",
		Cause:     cause,
	}
}

// Wrap wraps an error with additional context
func Wrap(err error, message string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

// WrapIf wraps an error with additional context if a condition is met
func WrapIf(condition bool, err error, message string) error {
	if !condition || err == nil {
		return err
	}
	return Wrap(err, message)
}

// As is a convenience function that wraps errors.As
func As(err error, target interface{}) bool {
	return errors.As(err, target)
}

// Is is a convenience function that wraps errors.Is
func Is(err, target error) bool {
	return errors.Is(err, target)
} 