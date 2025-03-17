package sentry

import (
	"context"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitialize(t *testing.T) {
	// Reset the initialized flag before each test
	initialized = false

	tests := []struct {
		name        string
		dsn         string
		environment string
		release     string
		wantErr     bool
	}{
		{
			name:        "successful initialization",
			dsn:         "https://test@sentry.io/123",
			environment: "test",
			release:     "1.0.0",
			wantErr:     false,
		},
		{
			name:        "empty DSN",
			dsn:         "",
			environment: "test",
			release:     "1.0.0",
			wantErr:     true,
		},
		{
			name:        "double initialization",
			dsn:         "https://test@sentry.io/123",
			environment: "test",
			release:     "1.0.0",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Initialize(tt.dsn, tt.environment, tt.release)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.True(t, initialized)
			}
		})
	}
}

func TestSetHubOnContext(t *testing.T) {
	// Reset the initialized flag
	initialized = false
	_ = Initialize("https://test@sentry.io/123", "test", "1.0.0")

	ctx := t.Context()
	newCtx := SetHubOnContext(ctx)

	assert.True(t, sentry.HasHubOnContext(newCtx))
	assert.NotNil(t, sentry.GetHubFromContext(newCtx))
}

func TestGetHubFromContext(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func() context.Context
		initialized bool
		wantNil     bool
	}{
		{
			name: "valid hub in context",
			setupFunc: func() context.Context {
				ctx := t.Context()
				return SetHubOnContext(ctx)
			},
			initialized: true,
			wantNil:     false,
		},
		{
			name: "no hub in context",
			setupFunc: func() context.Context {
				return t.Context()
			},
			initialized: true,
			wantNil:     true,
		},
		{
			name: "sentry not initialized",
			setupFunc: func() context.Context {
				return t.Context()
			},
			initialized: false,
			wantNil:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset the initialized flag
			initialized = false
			if tt.initialized {
				_ = Initialize("https://test@sentry.io/123", "test", "1.0.0")
			}

			ctx := tt.setupFunc()
			hub := getHubFromContext(ctx)

			if tt.wantNil {
				assert.Nil(t, hub)
			} else {
				assert.NotNil(t, hub)
			}
		})
	}
}

func TestSetTag(t *testing.T) {
	// Reset the initialized flag
	initialized = false
	_ = Initialize("https://test@sentry.io/123", "test", "1.0.0")

	tests := []struct {
		name      string
		setupFunc func() context.Context
		key       string
		value     string
	}{
		{
			name: "set tag with valid hub",
			setupFunc: func() context.Context {
				return SetHubOnContext(t.Context())
			},
			key:   "test-key",
			value: "test-value",
		},
		{
			name: "set tag with no hub",
			setupFunc: func() context.Context {
				return t.Context()
			},
			key:   "test-key",
			value: "test-value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupFunc()
			// This should not panic
			SetTag(ctx, tt.key, tt.value)
		})
	}
}

func TestCaptureError(t *testing.T) {
	// Reset the initialized flag
	initialized = false
	_ = Initialize("https://test@sentry.io/123", "test", "1.0.0")

	tests := []struct {
		name      string
		setupFunc func() context.Context
		err       error
	}{
		{
			name: "capture error with valid hub",
			setupFunc: func() context.Context {
				return SetHubOnContext(t.Context())
			},
			err: assert.AnError,
		},
		{
			name: "capture error with no hub",
			setupFunc: func() context.Context {
				return t.Context()
			},
			err: assert.AnError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupFunc()
			// This should not panic
			CaptureError(ctx, tt.err)
		})
	}
}
