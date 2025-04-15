package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	mcpServer "github.com/mark3labs/mcp-go/server"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
	"github.com/tuannvm/slack-mcp-client/internal/config"
	"github.com/tuannvm/slack-mcp-client/internal/handlers"
	"github.com/tuannvm/slack-mcp-client/internal/handlers/llm"
	"github.com/tuannvm/slack-mcp-client/internal/handlers/system"
)

// Server manages the MCP server endpoint.
type Server struct {
	cfg          *config.Config
	logger       *logging.Logger
	mcp          *mcpServer.MCPServer
	handlerRegistry *handlers.Registry
}

// NewServer creates and configures a new MCP server.
func NewServer(cfg *config.Config, logger *logging.Logger) (*Server, error) {
	logger.Info("Initializing MCP Server...")

	// Create a new MCP server instance
	mcpInstance := mcpServer.NewMCPServer(
		"slack-mcp-client",
		"0.1.0", // Server version
		mcpServer.WithLogging(),
	)

	// Create a handler registry
	registry := handlers.NewRegistry(logger)

	// Create server instance
	server := &Server{
		cfg:            cfg,
		logger:         logger,
		mcp:            mcpInstance,
		handlerRegistry: registry,
	}

	// Register all handlers
	if err := server.registerHandlers(); err != nil {
		return nil, fmt.Errorf("failed to register handlers: %w", err)
	}

	return server, nil
}

// registerHandlers registers all the available tool handlers
func (s *Server) registerHandlers() error {
	// System tools
	helloHandler := system.NewHelloHandler(s.logger)
	if err := s.registerHandler(helloHandler); err != nil {
		return err
	}

	// LLM tools
	openAIHandler := llm.NewOpenAIHandler(s.logger)
	if openAIHandler.IsConfigured() {
		if err := s.registerHandler(openAIHandler); err != nil {
			return err
		}
	} else {
		s.logger.Warn("OpenAI handler not configured, skipping registration")
	}

	ollamaHandler := llm.NewOllamaHandler(s.logger)
	if ollamaHandler.IsConfigured() {
		if err := s.registerHandler(ollamaHandler); err != nil {
			return err
		}
	} else {
		s.logger.Warn("Ollama handler not configured, skipping registration")
	}

	return nil
}

// registerHandler registers a single handler with both the registry and MCP server
func (s *Server) registerHandler(handler handlers.ToolHandler) error {
	// Add to our registry
	if err := s.handlerRegistry.Register(handler); err != nil {
		return err
	}

	// Register with MCP server
	tool := handler.GetToolDefinition()
	
	s.mcp.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handler.Handle(ctx, req)
	})

	s.logger.Info("Registered MCP tool: %s", handler.GetName())
	return nil
}

// Run starts the MCP HTTP server.
// This function will block until the server stops.
func (s *Server) Run(ctx context.Context, listenAddr string) error {
	s.logger.Info("Starting MCP server on %s", listenAddr)

	// Get the handler provided by the mcp-go library
	mcpHandler := mcpServer.NewSSEServer(s.mcp)
	s.logger.Debug("Obtained MCP handler from server.NewSSEServer")

	// Create the HTTP server, using the MCP handler directly at the root
	httpServer := &http.Server{
		Addr:        listenAddr,
		Handler:     mcpHandler,
		BaseContext: func(_ net.Listener) context.Context { return ctx },
	}
	s.logger.Debug("HTTP server struct created for address %s with MCP root handler", httpServer.Addr)

	// Channel to signal server startup errors
	errChan := make(chan error, 1)

	// Start the server in a goroutine
	go func() {
		s.logger.Debug("Attempting ListenAndServe on %s", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("ListenAndServe error: %v", err)
			errChan <- fmt.Errorf("failed to start MCP HTTP server: %w", err)
		} else {
			s.logger.Debug("ListenAndServe finished cleanly or closed")
			errChan <- nil
		}
		close(errChan)
	}()

	s.logger.Debug("Waiting for context cancellation or server error...")

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		s.logger.Info("Context cancelled. Initiating shutdown...")
		// Create a deadline context for shutdown
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("MCP server Shutdown error: %v", err)
			return fmt.Errorf("mcp server shutdown failed: %w", err)
		}
		s.logger.Info("Shutdown complete")
		<-errChan // Wait for goroutine exit
		return nil
	case err := <-errChan:
		if err != nil {
			s.logger.Error("Received error from server goroutine: %v", err)
			// Attempt a shutdown anyway, in case it partially started
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = httpServer.Shutdown(shutdownCtx)
			return err
		}
		s.logger.Info("Server stopped cleanly")
		return nil
	}
} 