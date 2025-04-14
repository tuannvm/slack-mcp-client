package mcp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tuannvm/slack-mcp-client/internal/config"
	"github.com/tuannvm/slack-mcp-client/internal/tools"
)

// Server manages the MCP server endpoint.
type Server struct {
	cfg *config.Config
	log *log.Logger
	mcp *server.MCPServer
}

// NewServer creates and configures a new MCP server.
func NewServer(cfg *config.Config, logger *log.Logger) (*Server, error) {
	logger.Println("Initializing MCP Server...")

	// Create a new MCP server instance
	mcpInstance := server.NewMCPServer(
		// TODO: Add server name and version from config?
		"slack-mcp-client",
		"0.0.1", // Example version
		server.WithLogging(),
		// Add other options like middleware, auth if needed
	)

	// Register the Ollama tool
	mcpInstance.AddTool(mcp.NewTool("ollama",
		mcp.WithDescription("Processes text using Ollama LLM"),
		mcp.WithString("model",
			mcp.Description("The Ollama model to use"),
			mcp.Required(),
		),
		mcp.WithString("prompt",
			mcp.Description("The prompt to send to Ollama"),
			mcp.Required(),
		),
		mcp.WithString("temperature",
			mcp.Description("Temperature for response generation"),
		),
		mcp.WithString("max_tokens",
			mcp.Description("Maximum number of tokens to generate"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return tools.HandleOllamaTool(ctx, req, logger)
	})
	logger.Printf("Registered MCP tool: ollama")

	// Register the hello tool
	mcpInstance.AddTool(mcp.NewTool("hello",
		mcp.WithDescription("Responds with a greeting."),
		mcp.WithString("name",
			mcp.Description("The name to say hello to. (Optional)"),
			// mcp.Required(), // Making name optional
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Pass the logger into the handler
		return tools.HandleHelloTool(ctx, req, logger)
	})
	logger.Printf("Registered MCP tool: hello")

	return &Server{
		cfg: cfg,
		log: logger,
		mcp: mcpInstance,
	}, nil
}

// Run starts the MCP HTTP server.
// This function will block until the server stops.
func (s *Server) Run(ctx context.Context, listenAddr string) error {
	s.log.Printf("Starting MCP server on %s (Run method entry)", listenAddr)

	// Get the handler provided by the mcp-go library
	mcpHandler := server.NewSSEServer(s.mcp)
	s.log.Printf("Obtained MCP handler from server.NewSSEServer")

	// Create the HTTP server, using the MCP handler directly at the root
	httpServer := &http.Server{
		Addr:        listenAddr,
		Handler:     mcpHandler, // Use the mcpHandler directly
		BaseContext: func(_ net.Listener) context.Context { return ctx }, // Propagate context for shutdown
	}
	s.log.Printf("HTTP server struct created for address %s with MCP root handler", httpServer.Addr)

	// Channel to signal server startup errors
	errChan := make(chan error, 1)

	// Start the server in a goroutine
	go func() {
		s.log.Printf("Goroutine: Attempting ListenAndServe on %s", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log.Printf("Goroutine: ListenAndServe error: %v", err)
			errChan <- fmt.Errorf("failed to start MCP HTTP server: %w", err)
		} else {
			s.log.Println("Goroutine: ListenAndServe finished cleanly or closed.")
			errChan <- nil
		}
		close(errChan)
		s.log.Println("Goroutine: Server goroutine finished.")
	}()

	s.log.Printf("Run method: Waiting for context cancellation or server error...")

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		s.log.Println("Run method: Context cancelled. Initiating shutdown...")
		// Create a deadline context for shutdown
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Give 10s for shutdown
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			s.log.Printf("MCP server Shutdown error: %v", err)
			return fmt.Errorf("mcp server shutdown failed: %w", err)
		}
		s.log.Println("Run method: Shutdown complete.")
		<-errChan // Wait for goroutine exit
		return nil
	case err := <-errChan:
		s.log.Printf("Run method: Received error from server goroutine: %v", err)
		if err != nil {
			// Attempt a shutdown anyway, in case it partially started
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = httpServer.Shutdown(shutdownCtx) // Ignore shutdown error here as we're returning the original failure
			s.log.Println("Run method: Returning server error.")
			return err // Return the original error
		}
		s.log.Println("Run method: Server stopped cleanly via errChan (nil error).")
		return nil // Or perhaps an unexpected stop error
	}
}
