package linode

import (
	"context"
	"fmt"

	"github.com/linode/linodego"
)

type vpcLookupError struct {
	value string
}

func (e vpcLookupError) Error() string {
	return fmt.Sprintf("failed to find VPC: %q", e.value)
}

// getVPCID returns the VPC id using the VPC label
func getVPCID(client Client, vpcName string) (int, error) {
	vpcs, err := client.ListVPCs(context.TODO(), &linodego.ListOptions{})
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
