package unit

import (
	"testing"

	"github.com/ksturgeon-td/mcp-over-socks/internal/config"
)

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *config.Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config with socks5",
			config: &config.Config{
				ProxyAddr: "socks5://localhost:1080",
				ServerURL: "http://example.com/sse",
				Timeout:   30,
				LogLevel:  "info",
			},
			wantErr: false,
		},
		{
			name: "valid config with socks5h (remote DNS)",
			config: &config.Config{
				ProxyAddr: "socks5h://localhost:1080",
				ServerURL: "http://example.com/sse",
				Timeout:   30,
				LogLevel:  "info",
			},
			wantErr: false,
		},
		{
			name: "valid config with https",
			config: &config.Config{
				ProxyAddr: "socks5://localhost:1080",
				ServerURL: "https://example.com/sse",
				Timeout:   30,
				LogLevel:  "info",
			},
			wantErr: false,
		},
		{
			name: "missing proxy address",
			config: &config.Config{
				ProxyAddr: "",
				ServerURL: "http://example.com/sse",
				Timeout:   30,
				LogLevel:  "info",
			},
			wantErr: true,
			errMsg:  "proxy address is required",
		},
		{
			name: "invalid proxy scheme",
			config: &config.Config{
				ProxyAddr: "http://localhost:1080",
				ServerURL: "http://example.com/sse",
				Timeout:   30,
				LogLevel:  "info",
			},
			wantErr: true,
			errMsg:  "proxy address must start with socks5:// or socks5h://",
		},
		{
			name: "missing server URL",
			config: &config.Config{
				ProxyAddr: "socks5://localhost:1080",
				ServerURL: "",
				Timeout:   30,
				LogLevel:  "info",
			},
			wantErr: true,
			errMsg:  "server URL is required",
		},
		{
			name: "invalid server scheme",
			config: &config.Config{
				ProxyAddr: "socks5://localhost:1080",
				ServerURL: "ftp://example.com/sse",
				Timeout:   30,
				LogLevel:  "info",
			},
			wantErr: true,
			errMsg:  "server URL must start with http:// or https://",
		},
		{
			name: "zero timeout",
			config: &config.Config{
				ProxyAddr: "socks5://localhost:1080",
				ServerURL: "http://example.com/sse",
				Timeout:   0,
				LogLevel:  "info",
			},
			wantErr: true,
			errMsg:  "timeout must be positive",
		},
		{
			name: "negative timeout",
			config: &config.Config{
				ProxyAddr: "socks5://localhost:1080",
				ServerURL: "http://example.com/sse",
				Timeout:   -1,
				LogLevel:  "info",
			},
			wantErr: true,
			errMsg:  "timeout must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
					return
				}
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestConfigProxyHost(t *testing.T) {
	tests := []struct {
		name      string
		proxyAddr string
		want      string
	}{
		{
			name:      "simple host:port",
			proxyAddr: "socks5://localhost:1080",
			want:      "localhost:1080",
		},
		{
			name:      "with user info",
			proxyAddr: "socks5://user:pass@localhost:1080",
			want:      "localhost:1080",
		},
		{
			name:      "ip address",
			proxyAddr: "socks5://192.168.1.1:1080",
			want:      "192.168.1.1:1080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{ProxyAddr: tt.proxyAddr}
			got := cfg.ProxyHost()
			if got != tt.want {
				t.Errorf("ProxyHost() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfigProxyAuth(t *testing.T) {
	tests := []struct {
		name      string
		proxyAddr string
		wantUser  string
		wantPass  string
		wantOK    bool
	}{
		{
			name:      "no auth",
			proxyAddr: "socks5://localhost:1080",
			wantUser:  "",
			wantPass:  "",
			wantOK:    false,
		},
		{
			name:      "with auth",
			proxyAddr: "socks5://user:pass@localhost:1080",
			wantUser:  "user",
			wantPass:  "pass",
			wantOK:    true,
		},
		{
			name:      "user only",
			proxyAddr: "socks5://user@localhost:1080",
			wantUser:  "user",
			wantPass:  "",
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{ProxyAddr: tt.proxyAddr}
			user, pass, ok := cfg.ProxyAuth()
			if user != tt.wantUser {
				t.Errorf("ProxyAuth() user = %q, want %q", user, tt.wantUser)
			}
			if pass != tt.wantPass {
				t.Errorf("ProxyAuth() pass = %q, want %q", pass, tt.wantPass)
			}
			if ok != tt.wantOK {
				t.Errorf("ProxyAuth() ok = %v, want %v", ok, tt.wantOK)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestConfigIsRemoteDNS(t *testing.T) {
	tests := []struct {
		name      string
		proxyAddr string
		want      bool
	}{
		{
			name:      "socks5 (local DNS)",
			proxyAddr: "socks5://localhost:1080",
			want:      false,
		},
		{
			name:      "socks5h (remote DNS)",
			proxyAddr: "socks5h://localhost:1080",
			want:      true,
		},
		{
			name:      "socks5h with auth",
			proxyAddr: "socks5h://user:pass@localhost:1080",
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{ProxyAddr: tt.proxyAddr}
			got := cfg.IsRemoteDNS()
			if got != tt.want {
				t.Errorf("IsRemoteDNS() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigProxyScheme(t *testing.T) {
	tests := []struct {
		name      string
		proxyAddr string
		want      string
	}{
		{
			name:      "socks5",
			proxyAddr: "socks5://localhost:1080",
			want:      "socks5",
		},
		{
			name:      "socks5h",
			proxyAddr: "socks5h://localhost:1080",
			want:      "socks5h",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{ProxyAddr: tt.proxyAddr}
			got := cfg.ProxyScheme()
			if got != tt.want {
				t.Errorf("ProxyScheme() = %q, want %q", got, tt.want)
			}
		})
	}
}
