package linode

import (
	"errors"
	"testing"

	"github.com/linode/linodego"
)

func TestParseProviderID(t *testing.T) {
	for _, tc := range []struct {
		name        string
		providerID  string
		expectedID  int
		errExpected bool
	}{
		{
			name:        "empty string is invalid",
			providerID:  "",
			errExpected: true,
		},
		{
			name:        "malformed provider id",
			providerID:  "invalidproviderid!",
			errExpected: true,
		},
		{
			name:        "wrong prefix",
			providerID:  "notlinode://123",
			errExpected: true,
		},
		{
			name:       "valid",
			providerID: "linode://123",
			expectedID: 123,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id, err := parseProviderID(tc.providerID)
			if err != nil {
				if !tc.errExpected {
					t.Errorf("unexpected error: %v", err)
				}
			} else if tc.errExpected {
				t.Error("expected an error; got nil")
			}

			if id != tc.expectedID {
				t.Errorf("expected id to be %d; got %d", tc.expectedID, id)
			}
		})
	}
}

func TestIgnoreLinodeAPIError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		err          error
		code         int
		shouldFilter bool
	}{{
		name:         "Not Linode API error",
		err:          errors.New("foo"),
		code:         0,
		shouldFilter: false,
	}, {
		name: "Ignore not found Linode API error",
		err: linodego.Error{
			Response: nil,
			Code:     400,
			Message:  "not found",
		},
		code:         400,
		shouldFilter: true,
	}, {
		name: "Don't ignore not found Linode API error",
		err: linodego.Error{
			Response: nil,
			Code:     400,
			Message:  "not found",
		},
		code:         500,
		shouldFilter: false,
	}}
	for _, tt := range tests {
		testcase := tt
		t.Run(testcase.name, func(t *testing.T) {
			t.Parallel()
			err := IgnoreLinodeAPIError(testcase.err, testcase.code)
			if testcase.shouldFilter && err != nil {
				t.Error("expected err but got nil")
			}
		})
	}
}
