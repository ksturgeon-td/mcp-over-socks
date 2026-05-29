// Package transport provides transport implementations for the MCP over SOCKS bridge.
package transport

import (
	"context"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/proxy"
)

// SOCKSDialer wraps a SOCKS5 proxy dialer.
type SOCKSDialer struct {
	dialer    proxy.Dialer
	remoteDNS bool // If true, let the proxy resolve hostnames (socks5h://)
}

// SOCKSError represents a SOCKS-related error with user-friendly message.
type SOCKSError struct {
	Message string
	Err     error
}

func (e *SOCKSError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *SOCKSError) Unwrap() error {
	return e.Err
}

// NewSOCKSDialer creates a new SOCKS5 dialer.
// proxyAddr should be in the format "host:port".
// auth can be nil for no authentication.
// remoteDNS specifies whether to let the proxy server resolve hostnames (socks5h://).
func NewSOCKSDialer(proxyAddr string, auth *proxy.Auth, remoteDNS bool) (*SOCKSDialer, error) {
	if proxyAddr == "" {
		return nil, &SOCKSError{
			Message: "SOCKS proxy address is empty",
		}
	}

	dialer, err := proxy.SOCKS5("tcp", proxyAddr, auth, proxy.Direct)
	if err != nil {
		return nil, &SOCKSError{
			Message: "Failed to create SOCKS5 dialer for " + proxyAddr,
			Err:     err,
		}
	}
	return &SOCKSDialer{
		dialer:    dialer,
		remoteDNS: remoteDNS,
	}, nil
}

// Dial connects to the address on the named network through the SOCKS5 proxy.
func (d *SOCKSDialer) Dial(network, addr string) (net.Conn, error) {
	dialAddr := addr
	if !d.remoteDNS {
		// For socks5://, resolve the hostname locally first
		resolved, err := d.resolveLocally(addr)
		if err != nil {
			return nil, err
		}
		dialAddr = resolved
	}
	// For socks5h://, pass the hostname as-is to let the proxy resolve it
	return d.dialer.Dial(network, dialAddr)
}

// DialContext connects to the address on the named network through the SOCKS5 proxy with context.
func (d *SOCKSDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	dialAddr := addr
	if !d.remoteDNS {
		// For socks5://, resolve the hostname locally first
		resolved, err := d.resolveLocallyWithContext(ctx, addr)
		if err != nil {
			return nil, err
		}
		dialAddr = resolved
	}
	// For socks5h://, pass the hostname as-is to let the proxy resolve it

	// Check if the dialer supports DialContext
	if ctxDialer, ok := d.dialer.(proxy.ContextDialer); ok {
		return ctxDialer.DialContext(ctx, network, dialAddr)
	}

	// Fallback: use channel to handle context cancellation
	type dialResult struct {
		conn net.Conn
		err  error
	}
	resultCh := make(chan dialResult, 1)

	go func() {
		conn, err := d.dialer.Dial(network, dialAddr)
		resultCh <- dialResult{conn: conn, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		return result.conn, result.err
	}
}

// resolveLocally resolves the hostname part of addr to an IP address.
// Returns the addr with hostname replaced by IP, or original addr if it's already an IP.
func (d *SOCKSDialer) resolveLocally(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, nil // Return as-is if parsing fails
	}

	// Check if it's already an IP address
	if ip := net.ParseIP(host); ip != nil {
		return addr, nil // Already an IP, no resolution needed
	}

	// Resolve the hostname
	ips, err := net.LookupHost(host)
	if err != nil {
		return "", &SOCKSError{
			Message: "Failed to resolve hostname '" + host + "' locally",
			Err:     err,
		}
	}
	if len(ips) == 0 {
		return "", &SOCKSError{
			Message: "No IP addresses found for hostname '" + host + "'",
		}
	}

	return net.JoinHostPort(ips[0], port), nil
}

// resolveLocallyWithContext is like resolveLocally but with context support.
func (d *SOCKSDialer) resolveLocallyWithContext(ctx context.Context, addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, nil // Return as-is if parsing fails
	}

	// Check if it's already an IP address
	if ip := net.ParseIP(host); ip != nil {
		return addr, nil // Already an IP, no resolution needed
	}

	// Resolve the hostname with context
	resolver := net.Resolver{}
	ips, err := resolver.LookupHost(ctx, host)
	if err != nil {
		return "", &SOCKSError{
			Message: "Failed to resolve hostname '" + host + "' locally",
			Err:     err,
		}
	}
	if len(ips) == 0 {
		return "", &SOCKSError{
			Message: "No IP addresses found for hostname '" + host + "'",
		}
	}

	return net.JoinHostPort(ips[0], port), nil
}

// IsRemoteDNS returns true if the dialer uses remote DNS resolution (socks5h://).
func (d *SOCKSDialer) IsRemoteDNS() bool {
	return d.remoteDNS
}

// HTTPTransport creates an http.Transport that uses this SOCKS5 dialer.
func (d *SOCKSDialer) HTTPTransport() *http.Transport {
	return &http.Transport{
		DialContext: d.DialContext,
	}
}

// HTTPClient creates an http.Client that uses this SOCKS5 dialer.
func (d *SOCKSDialer) HTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: d.HTTPTransport(),
		Timeout:   timeout,
	}
}

// headerTransport wraps an http.RoundTripper and injects static headers on every request.
type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqCopy := req.Clone(req.Context())
	for k, v := range t.headers {
		reqCopy.Header.Set(k, v)
	}
	return t.base.RoundTrip(reqCopy)
}

// HTTPClientWithHeaders creates an http.Client that uses the SOCKS dialer and injects static headers on every request.
func (d *SOCKSDialer) HTTPClientWithHeaders(timeout time.Duration, headers map[string]string) *http.Client {
	var rt http.RoundTripper = d.HTTPTransport()
	if len(headers) > 0 {
		rt = &headerTransport{base: rt, headers: headers}
	}
	return &http.Client{Transport: rt, Timeout: timeout}
}
