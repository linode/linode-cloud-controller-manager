package linode

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client"
	"github.com/linode/linodego"
	"k8s.io/klog/v2"
)

var (
	Mu sync.RWMutex
	// vpcIDs map stores vpc id's for given vpc labels
	vpcIDs = make(map[string]int, 0)
)

type vpcLookupError struct {
	value string
}

func (e vpcLookupError) Error() string {
	return fmt.Sprintf("failed to find VPC: %q", e.value)
}

// GetAllVPCIDs returns vpc ids stored in map
func GetAllVPCIDs() []int {
	Mu.Lock()
	defer Mu.Unlock()
	values := make([]int, 0, len(vpcIDs))
	for _, v := range vpcIDs {
		values = append(values, v)
	}
	return values
}

// GetVPCID returns the VPC id of given VPC label
func GetVPCID(ctx context.Context, client client.Client, vpcName string) (int, error) {
	Mu.Lock()
	defer Mu.Unlock()

	// check if map contains vpc id for given label
	if vpcid, ok := vpcIDs[vpcName]; ok {
		return vpcid, nil
	}
	vpcs, err := client.ListVPCs(ctx, &linodego.ListOptions{})
	if err != nil {
		return 0, err
	}
	for _, vpc := range vpcs {
		if vpc.Label == vpcName {
			vpcIDs[vpcName] = vpc.ID
			return vpc.ID, nil
		}
	}
	return 0, vpcLookupError{vpcName}
}

// GetVPCIPAddresses returns vpc ip's for given VPC label
func GetVPCIPAddresses(ctx context.Context, client client.Client, vpcName string) ([]linodego.VPCIP, error) {
	vpcID, err := GetVPCID(ctx, client, strings.TrimSpace(vpcName))
	if err != nil {
		return nil, err
	}
	resp, err := client.ListVPCIPAddresses(ctx, vpcID, linodego.NewListOptions(0, ""))
	if err != nil {
		if strings.Contains(err.Error(), "Not found") {
			Mu.Lock()
			defer Mu.Unlock()
			klog.Errorf("vpc %s not found. Deleting entry from cache", vpcName)
			delete(vpcIDs, vpcName)
		}
		return nil, err
	}
	return resp, nil
}
