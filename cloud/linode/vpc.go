package linode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/linode/linodego"
	"k8s.io/klog/v2"

	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client"
)

var (
	Mu sync.RWMutex
	// vpcIDs map stores vpc id's for given vpc labels
	vpcIDs = make(map[string]int, 0)
	// subnetIDs map stores subnet id's for given subnet labels
	subnetIDs = make(map[string]int, 0)
)

type vpcLookupError struct {
	value string
}

type subnetLookupError struct {
	value string
}

type subnetFilter struct {
	SubnetID string `json:"subnet_id"`
}

func (e vpcLookupError) Error() string {
	return fmt.Sprintf("failed to find VPC: %q", e.value)
}

func (e subnetLookupError) Error() string {
	return fmt.Sprintf("failed to find subnet: %q", e.value)
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

// GetSubnetID returns the subnet ID of given subnet label
func GetSubnetID(ctx context.Context, client client.Client, vpcID int, subnetName string) (int, error) {
	Mu.Lock()
	defer Mu.Unlock()

	// Check if map contains the id for the given label
	if subnetid, ok := subnetIDs[subnetName]; ok {
		return subnetid, nil
	}
	// Otherwise, get it from linodego.ListVPCSubnets()
	subnets, err := client.ListVPCSubnets(ctx, vpcID, &linodego.ListOptions{})
	if err != nil {
		return 0, err
	}
	for _, subnet := range subnets {
		if subnet.Label == subnetName {
			subnetIDs[subnetName] = subnet.ID
			return subnet.ID, nil
		}
	}

	return 0, subnetLookupError{subnetName}
}

// getVPCIDAndFilter returns the VPC ID and a resultFilter for subnet names (if any)
func getVPCIDAndFilter(ctx context.Context, client client.Client, vpcName string) (int, string, error) {
	vpcID, err := GetVPCID(ctx, client, strings.TrimSpace(vpcName))
	if err != nil {
		return 0, "", err
	}

	resultFilter := ""

	// Get subnet ID(s) from name(s) if subnet-names is specified
	if len(Options.SubnetNames) > 0 {
		// subnetIDList is a slice of strings for ease of use with resultFilter
		subnetIDList := []string{}

		for _, name := range Options.SubnetNames {
			// For caching
			var subnetID int
			subnetID, err = GetSubnetID(ctx, client, vpcID, name)
			// Don't filter subnets we can't find
			if err != nil {
				klog.Errorf("subnet %s not found due to error: %v. Skipping.", name, err)
				continue
			}

			// For use with the JSON filter
			subnetIDList = append(subnetIDList, strconv.Itoa(subnetID))
		}

		// Assign the list of IDs to a stringified JSON filter
		var filter []byte
		filter, err = json.Marshal(subnetFilter{SubnetID: strings.Join(subnetIDList, ",")})
		if err != nil {
			klog.Error("could not create JSON filter for subnet_id")
		} else {
			resultFilter = string(filter)
		}
	}

	return vpcID, resultFilter, nil
}

// handleNotFoundError checks if the error is a '404 Not Found error' and deletes the entry from the cache.
func handleNotFoundError(err error, vpcName string) error {
	if linodego.ErrHasStatus(err, http.StatusNotFound) {
		Mu.Lock()
		defer Mu.Unlock()
		klog.Errorf("vpc %s not found. Deleting entry from cache", vpcName)
		delete(vpcIDs, vpcName)
	}
	return err
}

// GetVPCIPAddresses returns vpc ip's for given VPC label
func GetVPCIPAddresses(ctx context.Context, client client.Client, vpcName string) ([]linodego.VPCIP, error) {
	vpcID, resultFilter, err := getVPCIDAndFilter(ctx, client, vpcName)
	if err != nil {
		return nil, err
	}

	resp, err := client.ListVPCIPAddresses(ctx, vpcID, linodego.NewListOptions(0, resultFilter))
	if err != nil {
		return nil, handleNotFoundError(err, vpcName)
	}
	return resp, nil
}

func GetVPCIPv6Addresses(ctx context.Context, client client.Client, vpcName string) ([]linodego.VPCIP, error) {
	vpcID, resultFilter, err := getVPCIDAndFilter(ctx, client, vpcName)
	if err != nil {
		return nil, err
	}

	resp, err := client.ListVPCIPv6Addresses(ctx, vpcID, linodego.NewListOptions(0, resultFilter))
	if err != nil {
		return nil, handleNotFoundError(err, vpcName)
	}
	return resp, nil
}

// getNodeBalancerBackendIPv4SubnetID returns the subnet ID for the NodeBalancer backend IPv4 subnet.
// It uses the first VPC name from Options.VPCNames to find the VPC ID and then retrieves the subnet ID
// for the NodeBalancer backend IPv4 subnet name specified in Options.NodeBalancerBackendIPv4SubnetName.
func getNodeBalancerBackendIPv4SubnetID(client client.Client) (int, error) {
	// Get the VPC ID from the name
	vpcID, err := GetVPCID(context.TODO(), client, Options.VPCNames[0])
	if err != nil {
		return 0, fmt.Errorf("failed to get vpc id for %s: %w", Options.VPCNames[0], err)
	}
	// Get the subnet ID from the name
	subnetID, err := GetSubnetID(context.TODO(), client, vpcID, Options.NodeBalancerBackendIPv4SubnetName)
	if err != nil {
		return 0, fmt.Errorf("failed to get subnet id for %s: %w", Options.NodeBalancerBackendIPv4SubnetName, err)
	}

	return subnetID, nil
}

// validateBothNameAndIDNotSet checks if both VPC IDs and names are set.
func validateBothNameAndIDNotSet() error {
	if len(Options.VPCIDs) != 0 && len(Options.VPCNames) != 0 {
		return fmt.Errorf("cannot have both vpc-ids and vpc-names set")
	}
	return nil
}

// validateVPCAndSubnetPairing checks if both VPC and subnet IDs or names are set correctly.
func validateVPCAndSubnetPairing() error {
	if len(Options.VPCIDs) != 0 && len(Options.SubnetIDs) == 0 {
		return fmt.Errorf("vpc-ids cannot be set without subnet-ids")
	}
	if len(Options.SubnetIDs) != 0 && len(Options.VPCIDs) == 0 {
		return fmt.Errorf("subnet-ids cannot be set without vpc-ids")
	}
	return nil
}

// validateSubnetNamesWithRouteController checks if subnet-names are set without vpc-names
func validateSubnetNamesWithRouteController() error {
	if len(Options.SubnetNames) != 0 && len(Options.VPCNames) == 0 && Options.EnableRouteController {
		return fmt.Errorf("subnet-names cannot be set without vpc-names")
	}
	return nil
}

// validateVPCSubnetFlags validates the VPC and subnet flags for the route controller.
// It checks that both VPC and subnet IDs or names are set correctly, and that they are paired correctly.
// It also checks that if subnet names are set, VPC names must also be set,
// and that if subnet IDs are set, VPC IDs must also be set.
func validateVPCSubnetFlags() error {
	if err := validateBothNameAndIDNotSet(); err != nil {
		return err
	}
	if err := validateVPCAndSubnetPairing(); err != nil {
		return err
	}
	if err := validateSubnetNamesWithRouteController(); err != nil {
		return err
	}
	return nil
}

// resolveSubnetNames resolves subnet ids to names for the given VPC ID.
func resolveSubnetNames(client client.Client, vpcID int) ([]string, error) {
	subnetNames := []string{}
	for _, subnetID := range Options.SubnetIDs {
		subnet, err := client.GetVPCSubnet(context.TODO(), vpcID, subnetID)
		if err != nil {
			return nil, fmt.Errorf("failed to get subnet %d for VPC %d: %w", subnetID, vpcID, err)
		}

		subnetIDs[subnet.Label] = subnet.ID
		subnetNames = append(subnetNames, subnet.Label)
	}

	return subnetNames, nil
}

// validateAndSetVPCSubnetFlags validates the VPC and subnet flags and sets the vpcNames and subnetNames options.
// It retrieves the VPC names and subnet names from the Linode API based on the provided flags.
// If subnet IDs are provided, it resolves the subnet names based on the first VPC ID.
func validateAndSetVPCSubnetFlags(client client.Client) error {
	// ignore default subnet-names if subnet-ids are set
	if len(Options.SubnetIDs) > 0 {
		Options.SubnetNames = []string{}
	}

	if err := validateVPCSubnetFlags(); err != nil {
		return err
	}

	Mu.Lock()
	defer Mu.Unlock()

	vpcNames := []string{}
	for idx, vpcID := range Options.VPCIDs {
		vpc, err := client.GetVPC(context.TODO(), vpcID)
		if err != nil {
			return fmt.Errorf("failed to get VPC %d: %w", vpcID, err)
		}

		vpcIDs[vpc.Label] = vpcID
		vpcNames = append(vpcNames, vpc.Label)

		// we resolve subnet names only for the first VPC ID
		// as there is no vpc to subnet mapping in input flags
		// and we assume all subnets are in the same VPC
		if idx == 0 && len(Options.SubnetIDs) > 0 {
			subnetNames, err := resolveSubnetNames(client, vpcID)
			if err != nil {
				return err
			}
			Options.SubnetNames = append(Options.SubnetNames, subnetNames...)
		}
	}

	Options.VPCNames = append(Options.VPCNames, vpcNames...)
	klog.V(3).Infof("VPC IDs: %v, VPC Names: %v, Subnet IDs: %v, Subnet Names: %v",
		Options.VPCIDs, Options.VPCNames, Options.SubnetIDs, Options.SubnetNames)
	return nil
}
