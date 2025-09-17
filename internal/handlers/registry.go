package handlers

import (
	"fmt"
	"sync"

	"github.com/tuannvm/slack-mcp-client/v2/internal/common/logging"
)

// Registry manages all available tool handlers
type Registry struct {
	handlers map[string]ToolHandler
	logger   *logging.Logger
	mu       sync.RWMutex
}

// NewRegistry creates a new handler registry
func NewRegistry(logger *logging.Logger) *Registry {
	return &Registry{
		handlers: make(map[string]ToolHandler),
		logger:   logger.WithName("handler-registry"),
	}
}

// Register adds a handler to the registry
func (r *Registry) Register(handler ToolHandler) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := handler.GetName()
	if _, exists := r.handlers[name]; exists {
		return fmt.Errorf("handler %s already registered", name)
	}

	r.handlers[name] = handler
	r.logger.Info("Registered handler: %s", name)
	return nil
}

// Get retrieves a handler by name
func (r *Registry) Get(name string) (ToolHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	handler, exists := r.handlers[name]
	return handler, exists
}

// GetAll returns all registered handlers
func (r *Registry) GetAll() []ToolHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()

	handlers := make([]ToolHandler, 0, len(r.handlers))
	for _, handler := range r.handlers {
		handlers = append(handlers, handler)
	}
	return handlers
}

// Unregister removes a handler from the registry
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.handlers[name]; exists {
		delete(r.handlers, name)
		r.logger.Info("Unregistered handler: %s", name)
	}
}
