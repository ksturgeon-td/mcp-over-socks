// Package bridge provides the MCP bridge between stdio and SSE/HTTP transport.
package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/ksturgeon-td/mcp-over-socks/internal/config"
	"github.com/ksturgeon-td/mcp-over-socks/internal/logging"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TransportType represents the type of transport to use.
type TransportType string

const (
	// TransportSSE uses the SSE transport (2024-11-05 spec).
	TransportSSE TransportType = "sse"
	// TransportStreamable uses the Streamable HTTP transport (2025-03-26 spec).
	TransportStreamable TransportType = "streamable"
)

// Bridge connects stdio to a remote MCP server using the official MCP SDK.
type Bridge struct {
	config        *config.Config
	logger        *logging.Logger
	httpClient    *http.Client
	transportType TransportType

	stdin  io.Reader
	stdout io.Writer
}

// New creates a new Bridge.
func New(cfg *config.Config, httpClient *http.Client, logger *logging.Logger, transportType TransportType) *Bridge {
	return &Bridge{
		config:        cfg,
		logger:        logger,
		httpClient:    httpClient,
		transportType: transportType,
		stdin:         os.Stdin,
		stdout:        os.Stdout,
	}
}

// NewWithIO creates a new Bridge with custom IO (for testing).
func NewWithIO(cfg *config.Config, httpClient *http.Client, logger *logging.Logger, transportType TransportType, stdin io.Reader, stdout io.Writer) *Bridge {
	return &Bridge{
		config:        cfg,
		logger:        logger,
		httpClient:    httpClient,
		transportType: transportType,
		stdin:         stdin,
		stdout:        stdout,
	}
}

// Run starts the bridge and blocks until the context is cancelled or an error occurs.
func (b *Bridge) Run(ctx context.Context) error {
	b.logger.Info("Connecting to MCP server: %s", b.config.ServerURL)
	b.logger.Debug("Using proxy: %s", b.config.ProxyAddr)
	b.logger.Debug("Transport type: %s", b.transportType)

	// Create the appropriate transport
	var transport mcp.Transport
	switch b.transportType {
	case TransportSSE:
		transport = &mcp.SSEClientTransport{
			Endpoint:   b.config.ServerURL,
			HTTPClient: b.httpClient,
		}
	case TransportStreamable:
		transport = &mcp.StreamableClientTransport{
			Endpoint:   b.config.ServerURL,
			HTTPClient: b.httpClient,
		}
	default:
		return fmt.Errorf("unknown transport type: %s", b.transportType)
	}

	// Connect to the server
	conn, err := transport.Connect(ctx)
	if err != nil {
		b.logger.Error("Connection failed: %v", err)
		return WrapError(ErrServerConnection, err.Error())
	}
	defer func() {
		b.logger.Info("Disconnecting from MCP server")
		conn.Close()
		b.logger.Debug("Connection closed")
	}()

	b.logger.Info("Connected to MCP server successfully")

	// Create channels for coordinating goroutines
	errCh := make(chan error, 2)
	var wg sync.WaitGroup

	// Start stdin reader goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := b.readStdin(ctx, conn); err != nil {
			select {
			case errCh <- fmt.Errorf("stdin reader error: %w", err):
			default:
			}
		}
	}()

	// Start response handler goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := b.handleResponses(ctx, conn); err != nil {
			select {
			case errCh <- fmt.Errorf("response handler error: %w", err):
			default:
			}
		}
	}()

	// Wait for context cancellation or error
	select {
	case <-ctx.Done():
		b.logger.Info("Shutting down bridge")
		return nil
	case err := <-errCh:
		return err
	}
}

// readStdin reads JSON-RPC requests from stdin and forwards them to the server.
func (b *Bridge) readStdin(ctx context.Context, conn mcp.Connection) error {
	scanner := bufio.NewScanner(b.stdin)
	// Increase buffer size for large JSON messages
	const maxScannerSize = 10 * 1024 * 1024 // 10MB
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, maxScannerSize)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Validate JSON
		if !json.Valid(line) {
			b.logger.Error("Invalid JSON received from stdin")
			continue
		}

		b.logger.Debug("Sending request to server: %s", string(line))

		// Parse the message using the SDK's jsonrpc package
		msg, err := jsonrpc.DecodeMessage(line)
		if err != nil {
			b.logger.Error("Failed to parse JSON-RPC message: %v", err)
			continue
		}

		// Write to the connection
		if err := conn.Write(ctx, msg); err != nil {
			b.logger.Error("Failed to send request: %v", err)
			// Send error response back to stdout
			b.sendErrorResponse(line, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stdin scanner error: %w", err)
	}

	return nil
}

// handleResponses reads responses from the connection and writes them to stdout.
func (b *Bridge) handleResponses(ctx context.Context, conn mcp.Connection) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		// Read from the connection with a timeout
		readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		msg, err := conn.Read(readCtx)
		cancel()

		if err != nil {
			if ctx.Err() != nil {
				return nil // Context cancelled, normal shutdown
			}
			if err == io.EOF {
				b.logger.Info("Connection closed by server")
				return nil
			}
			// Timeout is ok, just continue
			if readCtx.Err() == context.DeadlineExceeded {
				continue
			}
			b.logger.Error("Failed to read from connection: %v", err)
			return err
		}

		// Encode the message to JSON using the SDK's jsonrpc package
		data, err := jsonrpc.EncodeMessage(msg)
		if err != nil {
			b.logger.Error("Failed to encode response: %v", err)
			continue
		}

		b.logger.Debug("Received response from server: %s", string(data))

		// Write to stdout
		if _, err := fmt.Fprintln(b.stdout, string(data)); err != nil {
			return fmt.Errorf("failed to write to stdout: %w", err)
		}
	}
}

// sendErrorResponse sends a JSON-RPC error response to stdout.
func (b *Bridge) sendErrorResponse(request []byte, err error) {
	// Try to extract the request ID
	var req struct {
		ID interface{} `json:"id"`
	}
	json.Unmarshal(request, &req)

	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"error": map[string]interface{}{
			"code":    -32000,
			"message": err.Error(),
		},
	}

	data, _ := json.Marshal(response)
	fmt.Fprintln(b.stdout, string(data))
}
