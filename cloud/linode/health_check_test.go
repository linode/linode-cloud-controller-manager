package linode

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linode/linodego"
)

func TestHealthCheck(t *testing.T) {
	testCases := []struct {
		name string
		f    func(*testing.T, *linodego.Client, *fakeAPI)
	}{
		{
			name: "Test succeeding calls to linode api stop signal is not fired",
			f:    testSucceedingCallsToLinodeAPIHappenStopSignalNotFired,
		},
		{
			name: "Test failing calls to linode api stop signal is fired",
			f:    testFailingCallsToLinodeAPIHappenStopSignalFired,
		},
	}

	for _, tc := range testCases {
		fake := newFake(t)
		ts := httptest.NewServer(fake)

		linodeClient := linodego.NewClient(http.DefaultClient)
		linodeClient.SetBaseURL(ts.URL)

		t.Run(tc.name, func(t *testing.T) {
			defer ts.Close()
			tc.f(t, &linodeClient, fake)
		})
	}
}

func testSucceedingCallsToLinodeAPIHappenStopSignalNotFired(t *testing.T, client *linodego.Client, api *fakeAPI) {
	writableStopCh := make(chan struct{})
	readableStopCh := make(chan struct{})
	const validToken = "validtoken"

	api.tkn = validToken
	client.SetToken(validToken)

	hc, err := newHealthChecker(validToken, 1*time.Second, 1*time.Second, writableStopCh)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
	// inject modified linodego.Client
	hc.linodeClient = client

	go hc.Run(readableStopCh)

	// wait for check to happen
	time.Sleep(2 * time.Second)

	// stop healthChecker goroutine
	close(readableStopCh)

	if !api.didRequestOccur(http.MethodGet, "/profile", "") {
		t.Error("unexpected linode api calls")
		t.Logf("expected: %v /profile", http.MethodGet)
		t.Logf("actual: %v", api.requests)
	}

	select {
	case <-writableStopCh:
		t.Error("healthChecker sent stop signal")
	default:
	}
}

func testFailingCallsToLinodeAPIHappenStopSignalFired(t *testing.T, client *linodego.Client, api *fakeAPI) {
	writableStopCh := make(chan struct{})
	readableStopCh := make(chan struct{})
	const validToken = "validtoken"
	const invalidToken = "invalidtoken"

	api.tkn = validToken
	client.SetToken(validToken)

	hc, err := newHealthChecker(validToken, 1*time.Second, 1*time.Second, writableStopCh)
	if err != nil {
		t.Fatalf("expected a nil error, got %v", err)
	}
	// inject modified linodego.Client
	hc.linodeClient = client

	go hc.Run(readableStopCh)

	// wait for check to happen
	time.Sleep(2 * time.Second)

	if !api.didRequestOccur(http.MethodGet, "/profile", "") {
		t.Error("unexpected linode api calls")
		t.Logf("expected: %v /profile", http.MethodGet)
		t.Logf("actual: %v", api.requests)
	}

	select {
	case <-writableStopCh:
		t.Error("healthChecker sent stop signal")
	default:
	}

	// invalidate token
	api.tkn = invalidToken

	// wait for check to happen
	time.Sleep(2 * time.Second)

	select {
	case <-writableStopCh:
	default:
		t.Error("healthChecker did not send stop signal")
	}

	// stop healthChecker goroutine
	close(readableStopCh)
}
