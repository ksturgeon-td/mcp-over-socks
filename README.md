# MCP over SOCKS

A bridge that allows you to connect to SSE/Streamable HTTP MCP (Model Context Protocol) servers through a SOCKS5 proxy. This tool acts as a stdio MCP server locally, enabling Cursor and other MCP clients to access remote MCP servers that are only reachable via SOCKS proxy.

Built using the **official MCP Go SDK** ([github.com/modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk)).

## Features

- **SOCKS5 Proxy Support**: Connect to MCP servers through SOCKS5 proxies
  - `socks5://` - Local DNS resolution (resolve hostname before connecting)
  - `socks5h://` - Remote DNS resolution (let the proxy resolve the hostname)
- **Custom HTTP Headers**: Inject static headers on every request (e.g. `Authorization`, `X-Tenant`)
- **SSE Transport**: Full support for Server-Sent Events MCP transport
- **Streamable HTTP Transport**: Support for Streamable HTTP MCP transport
- **Auto-detection**: Automatically detects the transport type (SSE vs Streamable HTTP)
- **Single Binary**: No external dependencies at runtime
- **Cross-platform**: Works on macOS, Linux, and Windows

## Installation

### From Source

```bash
git clone https://github.com/ksturgeon-td/mcp-over-socks.git
cd mcp-over-socks
make build
```

### Using Go Install

```bash
go install github.com/ksturgeon-td/mcp-over-socks/cmd/mcp-over-socks@latest
```

## Usage

### Basic Usage

```bash
# Local DNS resolution (resolve hostname before connecting to proxy)
mcp-over-socks --proxy socks5://localhost:1080 --server http://mcp.example.com/sse

# Remote DNS resolution (let the proxy resolve the hostname)
mcp-over-socks --proxy socks5h://localhost:1080 --server http://internal-server.corp.local/sse
```

Use `socks5h://` when the MCP server hostname is only resolvable within the remote network (e.g., internal DNS names).

### Custom Headers

Use `--header` (repeatable) to inject static headers on every request — useful for bearer tokens, tenant IDs, or any other authentication scheme:

```bash
mcp-over-socks --proxy socks5://localhost:1080 --server http://mcp.example.com/sse \
  --header "Authorization: Bearer eyJhbGci..." \
  --header "X-Tenant: acme"
```

### Command Line Options

```
Usage: mcp-over-socks [options]

Required:
  --proxy      SOCKS5 proxy URL
               - socks5://host:port  (local DNS resolution)
               - socks5h://host:port (remote DNS resolution)
  --server     Remote MCP server URL (e.g., http://remote:8080/sse)

Optional:
  --timeout    Request timeout (default: 30s)
  --log        Log level: debug, info, error (default: info)
  --transport  Transport type: auto, sse, streamable (default: auto)
  --header     Custom HTTP header, repeatable (e.g. --header "Authorization: Bearer token")
  --version    Show version and exit
  --help       Show this help message
```

### Cursor Configuration

Add to your `~/.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "remote-mcp": {
      "command": "/path/to/mcp-over-socks",
      "args": [
        "--proxy", "socks5://localhost:1080",
        "--server", "http://your-mcp-server.example.com/sse"
      ]
    }
  }
}
```

With authentication headers:

```json
{
  "mcpServers": {
    "remote-mcp": {
      "command": "/path/to/mcp-over-socks",
      "args": [
        "--proxy", "socks5://localhost:1080",
        "--server", "http://your-mcp-server.example.com/sse",
        "--header", "Authorization: Bearer your-token-here"
      ]
    }
  }
}
```

### With SSH SOCKS Proxy

1. Start an SSH SOCKS proxy:
   ```bash
   ssh -D 1080 -N user@jump-host
   ```

2. Configure Cursor to use the bridge:
   ```json
   {
     "mcpServers": {
       "internal-mcp": {
         "command": "mcp-over-socks",
         "args": [
           "--proxy", "socks5h://127.0.0.1:1080",
           "--server", "http://internal-server.corp.example.com/mcp/sse"
         ]
       }
     }
   }
   ```

> **Note**: Use `socks5h://` when the internal server hostname is only resolvable from the jump host.

## Architecture

```
Cursor (MCP Client)
    ↓ stdio (JSON-RPC)
mcp-over-socks (Bridge)
    ↓ SOCKS5 Proxy
    ↓ HTTP/SSE or Streamable HTTP
Remote MCP Server
```

The bridge:
1. Receives JSON-RPC requests from Cursor via stdin
2. Forwards them to the remote MCP server through the SOCKS5 proxy
3. Returns responses to Cursor via stdout
4. Logs errors and status to stderr

## Development

### Building

```bash
make build          # Build for current platform
make build-all      # Build for all platforms
make test           # Run tests
make lint           # Run linter
make clean          # Clean build artifacts
```

### Running Tests

```bash
go test -v ./...
```

### Project Structure

```
.
├── cmd/
│   └── mcp-over-socks/
│       └── main.go          # Entry point
├── internal/
│   ├── bridge/
│   │   ├── bridge.go        # Main bridge logic (uses official MCP SDK)
│   │   └── errors.go        # Error types
│   ├── config/
│   │   └── config.go        # Configuration
│   ├── logging/
│   │   └── logger.go        # Logger
│   └── transport/
│       └── socks.go         # SOCKS5 dialer
├── tests/
│   └── unit/
├── Makefile
└── README.md
```

### Dependencies

- [github.com/modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk) - Official MCP Go SDK
- [golang.org/x/net/proxy](https://pkg.go.dev/golang.org/x/net/proxy) - SOCKS5 proxy support

## Troubleshooting

### Cannot connect to SOCKS proxy

1. Verify the SOCKS proxy is running
2. Check the proxy address format: `socks5://host:port` or `socks5h://host:port`
3. Ensure no firewall is blocking the connection

### Cannot resolve internal hostname

If you're connecting to an internal server with a hostname only resolvable within the remote network:
1. Use `socks5h://` instead of `socks5://` to enable remote DNS resolution
2. Verify the hostname is correct and resolvable from the proxy server

### Cannot connect to MCP server

1. Verify the MCP server is running and accessible through the proxy
2. Check the server URL format
3. Try connecting with `curl` through the proxy first:
   ```bash
   curl --proxy socks5://localhost:1080 http://mcp-server/sse
   ```

### Debug logging

Enable debug logging to see detailed information:

```bash
mcp-over-socks --proxy socks5://localhost:1080 --server http://example.com/sse --log debug 2>debug.log
```

## Known Limitations
- **No Automatic Reconnection**: If the SOCKS proxy connection is lost, the bridge will exit. You will need to restart the bridge (or rely on Cursor to restart it).
- **Authentication**: SOCKS5 authentication is supported via URL syntax: `socks5://user:password@host:port`.

## License

MIT License - see [LICENSE](LICENSE) for details.
