package linode

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/linode/linodego"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
)

const providerIDPrefix = "linode://"

type invalidProviderIDError struct {
	value string
}

func (e invalidProviderIDError) Error() string {
	return fmt.Sprintf("invalid provider ID %q", e.value)
}

func parseProviderID(providerID string) (int, error) {
	if !strings.HasPrefix(providerID, providerIDPrefix) {
		return 0, invalidProviderIDError{providerID}
	}
	id, err := strconv.Atoi(strings.TrimPrefix(providerID, providerIDPrefix))
	if err != nil {
		return 0, invalidProviderIDError{providerID}
	}
	return id, nil
}

func linodeFilterListOptions(targetLabel string) *linodego.ListOptions {
	jsonFilter := fmt.Sprintf(`{"label":%q}`, targetLabel)
	return linodego.NewListOptions(0, jsonFilter)
}

func linodeByName(ctx context.Context, client LinodeClient, nodeName types.NodeName) (*linodego.Instance, error) {
	linodes, err := client.ListInstances(ctx, linodeFilterListOptions(string(nodeName)))
	if err != nil {
		return nil, err
	}

	if len(linodes) == 0 {
		return nil, cloudprovider.InstanceNotFound
	} else if len(linodes) > 1 {
		return nil, errors.New(fmt.Sprintf("Multiple instances found with name %v", nodeName))
	}

	return &linodes[0], nil
}

func linodeByID(ctx context.Context, client LinodeClient, id int) (*linodego.Instance, error) {
	instance, err := client.GetInstance(ctx, id)
	if err != nil {
		return nil, err
	}
	if instance == nil {
		return nil, fmt.Errorf("linode not found with id %v", id)
	}
	return instance, nil
}
