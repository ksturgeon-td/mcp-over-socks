// Package config provides configuration management for the MCP over SOCKS bridge.
package config

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

// Config holds the configuration for the bridge.
type Config struct {
	// ProxyAddr is the SOCKS5 proxy address.
	// Supported schemes:
	//   - socks5://  - Local DNS resolution (resolve hostname locally before connecting)
	//   - socks5h:// - Remote DNS resolution (let the proxy server resolve the hostname)
	ProxyAddr string

	// ServerURL is the remote MCP server URL (e.g., "http://remote:8080/sse").
	ServerURL string

	// Timeout is the HTTP request timeout.
	Timeout time.Duration

	// LogLevel is the logging verbosity ("debug", "info", "error").
	LogLevel string

	// Headers are custom HTTP headers added to every request.
	Headers map[string]string
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() *Config {
	return &Config{
		Timeout:  30 * time.Second,
		LogLevel: "info",
	}
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.ProxyAddr == "" {
		return errors.New("proxy address is required (use --proxy)")
	}

	if !strings.HasPrefix(c.ProxyAddr, "socks5://") && !strings.HasPrefix(c.ProxyAddr, "socks5h://") {
		return errors.New("proxy address must start with socks5:// or socks5h://")
	}

	// Validate proxy URL format
	proxyURL, err := url.Parse(c.ProxyAddr)
	if err != nil {
		return errors.New("invalid proxy address format: " + err.Error())
	}
	if proxyURL.Host == "" {
		return errors.New("proxy address must include host")
	}

	if c.ServerURL == "" {
		return errors.New("server URL is required (use --server)")
	}

	if !strings.HasPrefix(c.ServerURL, "http://") && !strings.HasPrefix(c.ServerURL, "https://") {
		return errors.New("server URL must start with http:// or https://")
	}

	// Validate server URL format
	serverURL, err := url.Parse(c.ServerURL)
	if err != nil {
		return errors.New("invalid server URL format: " + err.Error())
	}
	if serverURL.Host == "" {
		return errors.New("server URL must include host")
	}

	if c.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}

	return nil
}

// ProxyHost returns the proxy host:port from the ProxyAddr.
func (c *Config) ProxyHost() string {
	u, err := url.Parse(c.ProxyAddr)
	if err != nil {
		return ""
	}
	return u.Host
}

// ProxyAuth returns the proxy authentication credentials if present.
func (c *Config) ProxyAuth() (username, password string, ok bool) {
	u, err := url.Parse(c.ProxyAddr)
	if err != nil || u.User == nil {
		return "", "", false
	}
	password, hasPassword := u.User.Password()
	return u.User.Username(), password, hasPassword
}

// IsRemoteDNS returns true if the proxy should perform DNS resolution (socks5h://).
func (c *Config) IsRemoteDNS() bool {
	return strings.HasPrefix(c.ProxyAddr, "socks5h://")
}

// ProxyScheme returns the proxy scheme ("socks5" or "socks5h").
func (c *Config) ProxyScheme() string {
	u, err := url.Parse(c.ProxyAddr)
	if err != nil {
		return ""
	}
	return u.Scheme
}
