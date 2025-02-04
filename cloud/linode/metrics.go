package linode

import (
	"sync"

	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client"

	"k8s.io/component-base/metrics/legacyregistry"
)

var registerOnce sync.Once

func registerMetrics() {
	registerOnce.Do(func() {
		legacyregistry.RawMustRegister(client.ClientMethodCounterVec)
	})
}
