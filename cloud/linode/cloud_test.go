package linode

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestNewCloudRouteControllerDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Setenv("LINODE_API_TOKEN", "dummyapitoken")
	t.Setenv("LINODE_REGION", "us-east")

	t.Run("should not fail if vpc is empty and routecontroller is disabled", func(t *testing.T) {
		Options.VPCName = ""
		Options.EnableRouteController = false
		_, err := newCloud()
		assert.NoError(t, err)
	})

	t.Run("fail if vpcname is empty and routecontroller is enabled", func(t *testing.T) {
		Options.VPCName = ""
		Options.EnableRouteController = true
		_, err := newCloud()
		assert.Error(t, err)
	})
}
