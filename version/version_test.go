package version

import "testing"

func TestVersion(t *testing.T) {
	t.Parallel()
	vers := GetVersion()
	if vers != "dev" {
		t.Errorf("unset version should be dev, got %s", vers)
	}
}
