package unit

import (
	"errors"
	"testing"

	"github.com/ksturgeon-td/mcp-over-socks/internal/bridge"
)

func TestBridgeError(t *testing.T) {
	tests := []struct {
		name    string
		err     *bridge.BridgeError
		wantMsg string
	}{
		{
			name: "with wrapped error",
			err: &bridge.BridgeError{
				Message: "outer message",
				Err:     errors.New("inner error"),
			},
			wantMsg: "outer message: inner error",
		},
		{
			name: "without wrapped error",
			err: &bridge.BridgeError{
				Message: "standalone message",
				Err:     nil,
			},
			wantMsg: "standalone message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.wantMsg {
				t.Errorf("BridgeError.Error() = %q, want %q", got, tt.wantMsg)
			}
		})
	}
}

func TestWrapError(t *testing.T) {
	t.Run("wraps error correctly", func(t *testing.T) {
		inner := errors.New("inner error")
		wrapped := bridge.WrapError(inner, "wrapped message")

		if wrapped == nil {
			t.Fatal("WrapError returned nil for non-nil error")
		}

		if wrapped.Error() != "wrapped message: inner error" {
			t.Errorf("unexpected error message: %q", wrapped.Error())
		}

		// Check unwrap
		var bridgeErr *bridge.BridgeError
		if !errors.As(wrapped, &bridgeErr) {
			t.Error("wrapped error is not a BridgeError")
		}
	})

	t.Run("returns nil for nil error", func(t *testing.T) {
		wrapped := bridge.WrapError(nil, "message")
		if wrapped != nil {
			t.Errorf("WrapError(nil) = %v, want nil", wrapped)
		}
	})
}

func TestErrorCheckers(t *testing.T) {
	t.Run("IsProxyError", func(t *testing.T) {
		proxyErr := bridge.WrapError(bridge.ErrProxyConnection, "test")
		if !bridge.IsProxyError(proxyErr) {
			t.Error("IsProxyError should return true for wrapped proxy error")
		}

		otherErr := errors.New("other error")
		if bridge.IsProxyError(otherErr) {
			t.Error("IsProxyError should return false for non-proxy error")
		}
	})

	t.Run("IsServerError", func(t *testing.T) {
		serverErr := bridge.WrapError(bridge.ErrServerConnection, "test")
		if !bridge.IsServerError(serverErr) {
			t.Error("IsServerError should return true for wrapped server error")
		}

		otherErr := errors.New("other error")
		if bridge.IsServerError(otherErr) {
			t.Error("IsServerError should return false for non-server error")
		}
	})

	t.Run("IsTimeoutError", func(t *testing.T) {
		timeoutErr := bridge.WrapError(bridge.ErrTimeout, "test")
		if !bridge.IsTimeoutError(timeoutErr) {
			t.Error("IsTimeoutError should return true for wrapped timeout error")
		}

		otherErr := errors.New("other error")
		if bridge.IsTimeoutError(otherErr) {
			t.Error("IsTimeoutError should return false for non-timeout error")
		}
	})
}

func TestFormatUserFriendlyError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantContain string
	}{
		{
			name:        "proxy error",
			err:         bridge.ErrProxyConnection,
			wantContain: "Cannot connect to SOCKS proxy",
		},
		{
			name:        "server error",
			err:         bridge.ErrServerConnection,
			wantContain: "Cannot connect to MCP server",
		},
		{
			name:        "timeout error",
			err:         bridge.ErrTimeout,
			wantContain: "Request timed out",
		},
		{
			name:        "config error",
			err:         bridge.ErrInvalidConfig,
			wantContain: "Invalid configuration",
		},
		{
			name:        "nil error",
			err:         nil,
			wantContain: "",
		},
		{
			name:        "other error",
			err:         errors.New("some other error"),
			wantContain: "some other error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bridge.FormatUserFriendlyError(tt.err)
			if tt.wantContain == "" && got != "" {
				t.Errorf("expected empty string, got %q", got)
				return
			}
			if tt.wantContain != "" && !containsString(got, tt.wantContain) {
				t.Errorf("FormatUserFriendlyError() = %q, want to contain %q", got, tt.wantContain)
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStringAt(s, substr, 0))
}

func containsStringAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
