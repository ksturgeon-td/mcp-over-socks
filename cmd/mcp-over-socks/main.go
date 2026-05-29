// Package main is the entry point for the MCP over SOCKS bridge.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ksturgeon-td/mcp-over-socks/internal/bridge"
	"github.com/ksturgeon-td/mcp-over-socks/internal/config"
	"github.com/ksturgeon-td/mcp-over-socks/internal/logging"
	"github.com/ksturgeon-td/mcp-over-socks/internal/transport"

	"golang.org/x/net/proxy"
)

const version = "0.2.0"

// headerFlags is a repeatable --header flag that accumulates "Name: Value" strings.
type headerFlags []string

func (h *headerFlags) String() string { return strings.Join(*h, ", ") }
func (h *headerFlags) Set(value string) error {
	*h = append(*h, value)
	return nil
}

func main() {
	// Define flags
	proxyAddr := flag.String("proxy", "", "SOCKS5 proxy URL (e.g., socks5://localhost:1080)")
	serverURL := flag.String("server", "", "Remote MCP server URL (e.g., http://remote:8080/sse)")
	timeout := flag.Duration("timeout", 30*time.Second, "Request timeout")
	logLevel := flag.String("log", "info", "Log level: debug, info, error")
	transportType := flag.String("transport", "auto", "Transport type: auto, sse, streamable")
	showVersion := flag.Bool("version", false, "Show version and exit")
	showHelp := flag.Bool("help", false, "Show help and exit")
	var rawHeaders headerFlags
	flag.Var(&rawHeaders, "header", "Custom HTTP header (repeatable, e.g. --header \"Authorization: Bearer token\")")

	// Custom usage function
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "mcp-over-socks - MCP bridge over SOCKS5 proxy\n\n")
		fmt.Fprintf(os.Stderr, "Uses the official MCP Go SDK (github.com/modelcontextprotocol/go-sdk)\n\n")
		fmt.Fprintf(os.Stderr, "Usage: mcp-over-socks [options]\n\n")
		fmt.Fprintf(os.Stderr, "Required:\n")
		fmt.Fprintf(os.Stderr, "  --proxy      SOCKS5 proxy URL:\n")
		fmt.Fprintf(os.Stderr, "               socks5://host:port  (local DNS resolution)\n")
		fmt.Fprintf(os.Stderr, "               socks5h://host:port (remote DNS resolution)\n")
		fmt.Fprintf(os.Stderr, "  --server     Remote MCP server URL (e.g., http://remote:8080/sse)\n\n")
		fmt.Fprintf(os.Stderr, "Optional:\n")
		fmt.Fprintf(os.Stderr, "  --timeout    Request timeout (default: 30s)\n")
		fmt.Fprintf(os.Stderr, "  --log        Log level: debug, info, error (default: info)\n")
		fmt.Fprintf(os.Stderr, "  --transport  Transport type: auto, sse, streamable (default: auto)\n")
		fmt.Fprintf(os.Stderr, "  --header     Custom HTTP header, repeatable (e.g. --header \"Authorization: Bearer token\")\n")
		fmt.Fprintf(os.Stderr, "  --version    Show version and exit\n")
		fmt.Fprintf(os.Stderr, "  --help       Show this help message\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  mcp-over-socks --proxy socks5://localhost:1080 --server http://mcp.example.com/sse\n")
		fmt.Fprintf(os.Stderr, "  mcp-over-socks --proxy socks5h://localhost:1080 --server http://internal.local/sse\n")
		fmt.Fprintf(os.Stderr, "  mcp-over-socks --proxy socks5://localhost:1080 --server http://mcp.example.com/sse --header \"Authorization: Bearer token\"\n")
	}

	flag.Parse()

	if *showHelp {
		flag.Usage()
		os.Exit(0)
	}

	if *showVersion {
		fmt.Printf("mcp-over-socks version %s\n", version)
		os.Exit(0)
	}

	// Create config
	cfg := &config.Config{
		ProxyAddr: *proxyAddr,
		ServerURL: *serverURL,
		Timeout:   *timeout,
		LogLevel:  *logLevel,
	}

	// Create logger
	logger := logging.New(logging.ParseLogLevel(cfg.LogLevel))

	// Parse custom headers
	headers := make(map[string]string, len(rawHeaders))
	for _, h := range rawHeaders {
		parts := strings.SplitN(h, ": ", 2)
		if len(parts) != 2 {
			logger.Error("Invalid header format %q (expected \"Name: Value\")", h)
			os.Exit(1)
		}
		headers[parts[0]] = parts[1]
	}
	cfg.Headers = headers

	// Validate config
	if err := cfg.Validate(); err != nil {
		logger.Error("Configuration error: %v", err)
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Run 'mcp-over-socks --help' for usage.")
		os.Exit(1)
	}

	// Create SOCKS dialer
	var auth *proxy.Auth
	if username, password, ok := cfg.ProxyAuth(); ok {
		auth = &proxy.Auth{
			User:     username,
			Password: password,
		}
	}

	socksDialer, err := transport.NewSOCKSDialer(cfg.ProxyHost(), auth, cfg.IsRemoteDNS())
	if err != nil {
		logger.Error("Failed to create SOCKS dialer: %v", err)
		os.Exit(1)
	}

	if cfg.IsRemoteDNS() {
		logger.Debug("Using remote DNS resolution (socks5h://)")
	} else {
		logger.Debug("Using local DNS resolution (socks5://)")
	}

	// Determine transport type
	tType := parseTransportType(*transportType, cfg.ServerURL)
	logger.Info("Using %s transport", tType)

	// Create HTTP client with SOCKS proxy
	httpClient := socksDialer.HTTPClientWithHeaders(cfg.Timeout, cfg.Headers)

	// Create bridge
	b := bridge.New(cfg, httpClient, logger, tType)

	// Setup context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logger.Info("Received shutdown signal")
		cancel()
	}()

	// Run bridge
	logger.Info("Starting MCP over SOCKS bridge")
	logger.Debug("Proxy: %s", cfg.ProxyAddr)
	logger.Debug("Server: %s", cfg.ServerURL)

	if err := b.Run(ctx); err != nil {
		logger.Error("Bridge error: %v", err)
		// Print user-friendly error message
		friendlyMsg := bridge.FormatUserFriendlyError(err)
		if friendlyMsg != "" && friendlyMsg != err.Error() {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, friendlyMsg)
		}
		os.Exit(1)
	}
}

// parseTransportType parses the transport type from string, with auto-detection based on URL.
func parseTransportType(s string, serverURL string) bridge.TransportType {
	switch strings.ToLower(s) {
	case "sse":
		return bridge.TransportSSE
	case "streamable", "streamablehttp", "streamable-http":
		return bridge.TransportStreamable
	default:
		// Auto-detect based on URL path
		// SSE endpoints typically end with /sse
		// Streamable HTTP endpoints typically end with /mcp
		if strings.HasSuffix(serverURL, "/sse") {
			return bridge.TransportSSE
		}
		if strings.HasSuffix(serverURL, "/mcp") {
			return bridge.TransportStreamable
		}
		// Default to SSE for backward compatibility
		return bridge.TransportSSE
	}
}
