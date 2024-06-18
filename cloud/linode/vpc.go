package linode

import (
	"context"
	"fmt"

	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client"
	"github.com/linode/linodego"
)

type vpcLookupError struct {
	value string
}

func (e vpcLookupError) Error() string {
	return fmt.Sprintf("failed to find VPC: %q", e.value)
}

// getVPCID returns the VPC id using the VPC label
func getVPCID(client client.Client, vpcName string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	vpcs, err := client.ListVPCs(ctx, &linodego.ListOptions{})
	if err != nil {
		return 0, err
	}
	for _, vpc := range vpcs {
		if vpc.Label == vpcName {
			return vpc.ID, nil
		}
	}
	return 0, vpcLookupError{vpcName}
}
