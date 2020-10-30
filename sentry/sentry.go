// Package sentry implements logic for using Sentry for error reporting.
package sentry

import (
	"context"
	"fmt"

	"github.com/getsentry/sentry-go"
	"k8s.io/klog/v2"
)

var initialized bool

// Initialize initializes a Sentry connection with the given client option values.
func Initialize(dsn, environment, release string) error {
	if initialized {
		return fmt.Errorf("sentry Initialize called after initialization")
	}

	var clientOptions sentry.ClientOptions

	clientOptions.Dsn = dsn
	clientOptions.Environment = environment
	clientOptions.Release = release

	if err := sentry.Init(clientOptions); err != nil {
		return err
	}

	initialized = true

	return nil
}

// SetHubOnContext clones the current hub and sets it on the given context.
func SetHubOnContext(ctx context.Context) context.Context {
	hub := sentry.CurrentHub().Clone()

	return sentry.SetHubOnContext(ctx, hub)
}

// getHubFromContext gets the current Sentry hub from the given context. If the context is missing a
// Sentry hub, this function logs an error and returns nil. If Sentry has not been initialized, this
// function also returns nil.
func getHubFromContext(ctx context.Context) *sentry.Hub {
	if !initialized {
		klog.V(5).Info("getHubFromContext: Sentry not initialized")
		return nil
	}

	if !sentry.HasHubOnContext(ctx) {
		klog.Error("getHubFromContext: context is missing Sentry hub")
		return nil
	}

	return sentry.GetHubFromContext(ctx)
}

// SetTag sets a tag for the hub associated with the given context. If Sentry is not enabled or the
// context has no associated hub, this function will have no effect.
func SetTag(ctx context.Context, key, value string) {
	hub := getHubFromContext(ctx)

	if hub == nil {
		return
	}

	hub.Scope().SetTag(key, value)
}

// CaptureError captures the current error and sends it to Sentry using the hub from the current
// context. This should only be used for actionable errors to avoid flooding Sentry with useless
// reports. If Sentry is not enabled or the context has no associated hub, this function will
// have no effect.
func CaptureError(ctx context.Context, err error) {
	hub := getHubFromContext(ctx)

	if hub == nil {
		return
	}

	hub.CaptureException(err)
}
