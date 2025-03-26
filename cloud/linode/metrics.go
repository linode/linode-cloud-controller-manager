package linode

import (
	"sync"

	"k8s.io/component-base/metrics/legacyregistry"

	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client"
)

var registerOnce sync.Once

func registerMetrics() {
	registerOnce.Do(func() {
		legacyregistry.RawMustRegister(client.ClientMethodCounterVec)
	})
}
