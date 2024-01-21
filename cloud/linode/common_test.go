package linode

import "testing"

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
