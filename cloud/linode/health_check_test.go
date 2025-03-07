package linode

import (
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/linode/linodego"

	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client/mocks"
)

func TestHealthCheck(t *testing.T) {
	testCases := []struct {
		name string
		f    func(*testing.T, *mocks.MockClient)
	}{
		{
			name: "Test succeeding calls to linode api stop signal is not fired",
			f:    testSucceedingCallsToLinodeAPIHappenStopSignalNotFired,
		},
		{
			name: "Test Unauthorized calls to linode api stop signal is fired",
			f:    testFailingCallsToLinodeAPIHappenStopSignalFired,
		},
		{
			name: "Test failing calls to linode api stop signal is not fired",
			f:    testErrorCallsToLinodeAPIHappenStopSignalNotFired,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			client := mocks.NewMockClient(ctrl)
			tc.f(t, client)
		})
	}
}

func testSucceedingCallsToLinodeAPIHappenStopSignalNotFired(t *testing.T, client *mocks.MockClient) {
	t.Helper()

	writableStopCh := make(chan struct{})
	readableStopCh := make(chan struct{})

	client.EXPECT().GetProfile(gomock.Any()).Times(2).Return(&linodego.Profile{}, nil)

	hc := newHealthChecker(client, 1*time.Second, writableStopCh)

	defer close(readableStopCh)
	go hc.Run(readableStopCh)

	// wait for two checks to happen
	time.Sleep(1500 * time.Millisecond)

	select {
	case <-writableStopCh:
		t.Error("healthChecker sent stop signal")
	default:
	}
}

func testFailingCallsToLinodeAPIHappenStopSignalFired(t *testing.T, client *mocks.MockClient) {
	t.Helper()

	writableStopCh := make(chan struct{})
	readableStopCh := make(chan struct{})

	client.EXPECT().GetProfile(gomock.Any()).Times(1).Return(&linodego.Profile{}, nil)

	hc := newHealthChecker(client, 1*time.Second, writableStopCh)

	defer close(readableStopCh)
	go hc.Run(readableStopCh)

	// wait for check to happen
	time.Sleep(500 * time.Millisecond)

	select {
	case <-writableStopCh:
		t.Error("healthChecker sent stop signal")
	default:
	}

	// invalidate token
	client.EXPECT().GetProfile(gomock.Any()).Times(1).Return(&linodego.Profile{}, &linodego.Error{Code: 401, Message: "Invalid Token"})

	// wait for check to happen
	time.Sleep(1 * time.Second)

	select {
	case <-writableStopCh:
	default:
		t.Error("healthChecker did not send stop signal")
	}
}

func testErrorCallsToLinodeAPIHappenStopSignalNotFired(t *testing.T, client *mocks.MockClient) {
	t.Helper()

	writableStopCh := make(chan struct{})
	readableStopCh := make(chan struct{})

	client.EXPECT().GetProfile(gomock.Any()).Times(1).Return(&linodego.Profile{}, nil)

	hc := newHealthChecker(client, 1*time.Second, writableStopCh)

	defer close(readableStopCh)
	go hc.Run(readableStopCh)

	// wait for check to happen
	time.Sleep(500 * time.Millisecond)

	select {
	case <-writableStopCh:
		t.Error("healthChecker sent stop signal")
	default:
	}

	// simulate server error
	client.EXPECT().GetProfile(gomock.Any()).Times(1).Return(&linodego.Profile{}, &linodego.Error{Code: 500})

	// wait for check to happen
	time.Sleep(1 * time.Second)

	select {
	case <-writableStopCh:
		t.Error("healthChecker sent stop signal")
	default:
	}

	client.EXPECT().GetProfile(gomock.Any()).Times(1).Return(&linodego.Profile{}, nil)

	// wait for check to happen
	time.Sleep(1 * time.Second)

	select {
	case <-writableStopCh:
		t.Error("healthChecker sent stop signal")
	default:
	}
}
