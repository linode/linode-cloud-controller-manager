package firewall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/linode/linodego"
	"golang.org/x/exp/slices"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/linode/linode-cloud-controller-manager/cloud/annotations"
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client"
)

const (
	maxFirewallRuleLabelLen = 32
	maxFirewallRuleDescLen  = 100
	maxIPsPerFirewall       = 255
	maxRulesPerFirewall     = 25
	accept                  = "ACCEPT"
	drop                    = "DROP"
	portRangeParts          = 2
)

var (
	ErrTooManyIPs         = errors.New("too many IPs in this ACL, will exceed rules per firewall limit")
	ErrTooManyNBFirewalls = errors.New("too many firewalls attached to a nodebalancer")
	ErrInvalidFWConfig    = errors.New("specify either an allowList or a denyList for a firewall")
)

type LinodeClient struct {
	Client client.Client
}

type aclConfig struct {
	AllowList *linodego.NetworkAddresses `json:"allowList"`
	DenyList  *linodego.NetworkAddresses `json:"denyList"`
}

func (l *LinodeClient) CreateFirewall(ctx context.Context, opts linodego.FirewallCreateOptions) (fw *linodego.Firewall, err error) {
	return l.Client.CreateFirewall(ctx, opts)
}

func (l *LinodeClient) DeleteFirewall(ctx context.Context, firewall *linodego.Firewall) error {
	fwDevices, err := l.Client.ListFirewallDevices(ctx, firewall.ID, &linodego.ListOptions{})
	if err != nil {
		klog.Errorf("Error in listing firewall devices: %v", err)
		return err
	}
	if len(fwDevices) > 1 {
		klog.Errorf("Found more than one device attached to firewall ID: %d, devices: %+v. Skipping delete of firewall", firewall.ID, fwDevices)
		return nil
	}
	return l.Client.DeleteFirewall(ctx, firewall.ID)
}

func (l *LinodeClient) DeleteNodeBalancerFirewall(
	ctx context.Context,
	service *v1.Service,
	nb *linodego.NodeBalancer,
) error {
	_, fwACLExists := service.GetAnnotations()[annotations.AnnLinodeCloudFirewallACL]
	if fwACLExists { // if an ACL exists, check if firewall exists and delete it.
		firewalls, err := l.Client.ListNodeBalancerFirewalls(ctx, nb.ID, &linodego.ListOptions{})
		if err != nil {
			return err
		}

		switch len(firewalls) {
		case 0:
			klog.Info("No firewall attached to nodebalancer, nothing to clean")
		case 1:
			return l.DeleteFirewall(ctx, &firewalls[0])
		default:
			klog.Errorf("Found more than one firewall attached to nodebalancer: %d, firewall IDs: %v", nb.ID, firewalls)
			return ErrTooManyNBFirewalls
		}
	}

	return nil
}

func ipsChanged(ips *linodego.NetworkAddresses, rules []linodego.FirewallRule) bool {
	var ruleIPv4s []string
	var ruleIPv6s []string

	for _, rule := range rules {
		if rule.Addresses.IPv4 != nil {
			ruleIPv4s = append(ruleIPv4s, *rule.Addresses.IPv4...)
		}
		if rule.Addresses.IPv6 != nil {
			ruleIPv6s = append(ruleIPv6s, *rule.Addresses.IPv6...)
		}
	}

	if len(ruleIPv4s) > 0 && ips.IPv4 == nil {
		return true
	}

	if len(ruleIPv6s) > 0 && ips.IPv6 == nil {
		return true
	}

	if ips.IPv4 != nil {
		if len(*ips.IPv4) != len(ruleIPv4s) {
			return true
		}
		for _, ipv4 := range *ips.IPv4 {
			if !slices.Contains(ruleIPv4s, ipv4) {
				return true
			}
		}
	}

	if ips.IPv6 != nil {
		if len(*ips.IPv6) != len(ruleIPv6s) {
			return true
		}
		for _, ipv6 := range *ips.IPv6 {
			if !slices.Contains(ruleIPv6s, ipv6) {
				return true
			}
		}
	}

	return false
}

func parsePorts(ports string) ([]int32, error) {
	var result []int32
	portParts := strings.Split(ports, ",")
	for _, part := range portParts {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != portRangeParts {
				return nil, fmt.Errorf("invalid range format: %s", part)
			}

			start, err1 := strconv.ParseInt(rangeParts[0], 10, 32)
			end, err2 := strconv.ParseInt(rangeParts[1], 10, 32)
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("invalid number in range: %s", part)
			}
			if start > end {
				return nil, fmt.Errorf("start greater than end in range: %s", part)
			}

			for i := start; i <= end; i++ {
				result = append(result, int32(i))
			}
		} else {
			port, err := strconv.ParseInt(part, 10, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid port: %s", part)
			}
			result = append(result, int32(port))
		}
	}

	return result, nil
}

func isPortsChanged(rules []linodego.FirewallRule, service *v1.Service) bool {
	// Service has at least one port, so we can check if there are any rules in firewall
	// We only care about the first rule, as all rules should have same ports
	if len(rules) == 0 {
		return true
	}
	oldPorts := rules[0].Ports
	if oldPorts == "" {
		return true
	}
	// Split the old ports by comma and convert to a slice of strings
	oldPortsSlice, err := parsePorts(oldPorts)
	if err != nil {
		klog.Errorf("Error parsing old ports: %v", err)
		return true
	}
	// Create a map to store the old ports for easy lookup
	oldPortsMap := make(map[int32]struct{}, len(oldPortsSlice))
	for _, port := range oldPortsSlice {
		oldPortsMap[port] = struct{}{}
	}
	svcPortsMap := make(map[int32]struct{}, len(service.Spec.Ports))
	for _, port := range service.Spec.Ports {
		svcPortsMap[port.Port] = struct{}{}
	}

	// Check if the ports in the service are different from the old ports
	for _, port := range service.Spec.Ports {
		if _, ok := oldPortsMap[port.Port]; !ok {
			return true
		}
	}

	// Check if there are any old ports that are not in the service
	for _, port := range oldPortsSlice {
		if _, ok := svcPortsMap[port]; !ok {
			return true
		}
	}

	// If all ports match, return false
	return false
}

// ruleChanged takes an old FirewallRuleSet and new aclConfig and returns if
// the IPs of the FirewallRuleSet would be changed with the new ACL Config
func ruleChanged(old linodego.FirewallRuleSet, newACL aclConfig, service *v1.Service) bool {
	var ips *linodego.NetworkAddresses
	if newACL.AllowList != nil {
		// this is a allowList, this means that the rules should have `DROP` as inboundpolicy
		if old.InboundPolicy != drop {
			return true
		}
		if (newACL.AllowList.IPv4 != nil || newACL.AllowList.IPv6 != nil) && len(old.Inbound) == 0 {
			return true
		}
		ips = newACL.AllowList
	}

	if newACL.DenyList != nil {
		if old.InboundPolicy != accept {
			return true
		}

		if (newACL.DenyList.IPv4 != nil || newACL.DenyList.IPv6 != nil) && len(old.Inbound) == 0 {
			return true
		}
		ips = newACL.DenyList
	}

	return ipsChanged(ips, old.Inbound) || isPortsChanged(old.Inbound, service)
}

func chunkIPs(ips []string) [][]string {
	chunks := [][]string{}
	ipCount := len(ips)

	// If the number of IPs is less than or equal to maxIPsPerFirewall,
	// return a single chunk containing all IPs.
	if ipCount <= maxIPsPerFirewall {
		return [][]string{ips}
	}

	// Otherwise, break the IPs into chunks with maxIPsPerFirewall IPs per chunk.
	chunkCount := 0
	for ipCount > maxIPsPerFirewall {
		start := chunkCount * maxIPsPerFirewall
		end := (chunkCount + 1) * maxIPsPerFirewall
		chunks = append(chunks, ips[start:end])
		chunkCount++
		ipCount -= maxIPsPerFirewall
	}

	// Append the remaining IPs as a chunk.
	chunks = append(chunks, ips[chunkCount*maxIPsPerFirewall:])

	return chunks
}

// truncateFWRuleDesc truncates the description to maxFirewallRuleDescLen if it exceeds the limit.
func truncateFWRuleDesc(desc string) string {
	if len(desc) > maxFirewallRuleDescLen {
		newDesc := desc[0:maxFirewallRuleDescLen-3] + "..."
		klog.Infof("Firewall rule description '%s' is too long. Stripping it to '%s'", desc, newDesc)
		desc = newDesc
	}
	return desc
}

// processACL takes the IPs, aclType, label etc and formats them into the passed linodego.FirewallCreateOptions pointer.
func processACL(fwcreateOpts *linodego.FirewallCreateOptions, aclType, label, svcName, ports string, ips linodego.NetworkAddresses) error {
	ruleLabel := fmt.Sprintf("%s-%s", aclType, svcName)
	if len(ruleLabel) > maxFirewallRuleLabelLen {
		newLabel := ruleLabel[0:maxFirewallRuleLabelLen]
		klog.Infof("Firewall label '%s' is too long. Stripping to '%s'", ruleLabel, newLabel)
		ruleLabel = newLabel
	}

	// Linode has a limitation of firewall rules with a max of 255 IPs per rule
	var ipv4s, ipv6s []string // doing this to avoid dereferencing a nil pointer
	if ips.IPv6 != nil {
		ipv6s = *ips.IPv6
	}
	if ips.IPv4 != nil {
		ipv4s = *ips.IPv4
	}

	if len(ipv4s)+len(ipv6s) > maxIPsPerFirewall {
		ipv4chunks := chunkIPs(ipv4s)
		for i, chunk := range ipv4chunks {
			v4chunk := chunk
			desc := fmt.Sprintf("Rule %d, Created by linode-ccm: %s, for %s", i, label, svcName)
			fwcreateOpts.Rules.Inbound = append(fwcreateOpts.Rules.Inbound, linodego.FirewallRule{
				Action:      aclType,
				Label:       ruleLabel,
				Description: truncateFWRuleDesc(desc),
				Protocol:    linodego.TCP, // Nodebalancers support only TCP.
				Ports:       ports,
				Addresses:   linodego.NetworkAddresses{IPv4: &v4chunk},
			})
		}

		ipv6chunks := chunkIPs(ipv6s)
		for i, chunk := range ipv6chunks {
			v6chunk := chunk
			desc := fmt.Sprintf("Rule %d, Created by linode-ccm: %s, for %s", i, label, svcName)
			fwcreateOpts.Rules.Inbound = append(fwcreateOpts.Rules.Inbound, linodego.FirewallRule{
				Action:      aclType,
				Label:       ruleLabel,
				Description: truncateFWRuleDesc(desc),
				Protocol:    linodego.TCP, // Nodebalancers support only TCP.
				Ports:       ports,
				Addresses:   linodego.NetworkAddresses{IPv6: &v6chunk},
			})
		}
	} else {
		desc := fmt.Sprintf("Created by linode-ccm: %s, for %s", label, svcName)
		fwcreateOpts.Rules.Inbound = append(fwcreateOpts.Rules.Inbound, linodego.FirewallRule{
			Action:      aclType,
			Label:       ruleLabel,
			Description: truncateFWRuleDesc(desc),
			Protocol:    linodego.TCP, // Nodebalancers support only TCP.
			Ports:       ports,
			Addresses:   ips,
		})
	}

	fwcreateOpts.Rules.OutboundPolicy = accept
	if aclType == accept {
		// if an allowlist is present, we drop everything else.
		fwcreateOpts.Rules.InboundPolicy = drop
	} else {
		// if a denylist is present, we accept everything else.
		fwcreateOpts.Rules.InboundPolicy = accept
	}

	if len(fwcreateOpts.Rules.Inbound) > maxRulesPerFirewall {
		return ErrTooManyIPs
	}
	return nil
}

// UpdateNodeBalancerFirewall reconciles the firewall attached to the nodebalancer
//
// This function does the following
//  1. If a firewallID annotation is present, it checks if the nodebalancer has a firewall attached, and if it matches the annotationID
//     a. If the IDs match, nothing to do here.
//     b. If they don't match, the nb is attached to the new firewall and removed from the old one.
//  2. If a firewallACL annotation is present,
//     a. it checks if the nodebalancer has a firewall attached, if a fw exists, it updates rules
//     b. if a fw does not exist, it creates one
//  3. If neither of these annotations are present,
//     a. AND if no firewalls are attached to the nodebalancer, nothing to do.
//     b. if the NB has ONE firewall attached, remove it from nb, and clean up if nothing else is attached to it
//     c. If there are more than one fw attached to it, then its a problem, return an err
//  4. If both these annotations are present, the firewallID takes precedence, and the ACL annotation is ignored.
//
// IF a user creates a fw ID externally, and then switches to using a ACL, the CCM will take over the fw that's attached to the nodebalancer.
func (l *LinodeClient) UpdateNodeBalancerFirewall(
	ctx context.Context,
	loadBalancerName string,
	loadBalancerTags []string,
	service *v1.Service,
	nb *linodego.NodeBalancer,
) error {
	// get the new firewall id from the annotation (if any).
	_, fwIDExists := service.GetAnnotations()[annotations.AnnLinodeCloudFirewallID]
	if fwIDExists { // If an ID exists, we ignore everything else and handle just that
		return l.updateServiceFirewall(ctx, service, nb)
	}

	// See if a acl exists
	_, fwACLExists := service.GetAnnotations()[annotations.AnnLinodeCloudFirewallACL]
	if fwACLExists { // if an ACL exists, but no ID, just update the ACL on the fw.
		return l.updateNodeBalancerFirewallWithACL(ctx, loadBalancerName, loadBalancerTags, service, nb)
	}

	// No firewall ID or ACL annotation, see if there are firewalls attached to our nb
	firewalls, err := l.Client.ListNodeBalancerFirewalls(ctx, nb.ID, &linodego.ListOptions{})
	if err != nil {
		return err
	}

	if len(firewalls) == 0 {
		return nil
	}
	if len(firewalls) > 1 {
		klog.Errorf("Found more than one firewall attached to nodebalancer: %d, firewall IDs: %v", nb.ID, firewalls)
		return ErrTooManyNBFirewalls
	}
	deviceID, deviceExists, err := l.getNodeBalancerDeviceID(ctx, firewalls[0].ID, nb.ID)
	if err != nil {
		return err
	}
	if deviceExists {
		err = l.Client.DeleteFirewallDevice(ctx, firewalls[0].ID, deviceID)
		if err != nil {
			return err
		}
	}

	// once we delete the device, we should see if there's anything attached to that firewall
	devices, err := l.Client.ListFirewallDevices(ctx, firewalls[0].ID, &linodego.ListOptions{})
	if err != nil {
		return err
	}

	if len(devices) == 0 {
		// nothing attached to it, clean it up
		return l.Client.DeleteFirewall(ctx, firewalls[0].ID)
	}
	// else let that firewall linger, don't mess with it.

	return nil
}

// getNodeBalancerDeviceID gets the deviceID of the nodeBalancer that is attached to the firewall.
func (l *LinodeClient) getNodeBalancerDeviceID(ctx context.Context, firewallID, nbID int) (int, bool, error) {
	devices, err := l.Client.ListFirewallDevices(ctx, firewallID, &linodego.ListOptions{})
	if err != nil {
		return 0, false, err
	}

	if len(devices) == 0 {
		return 0, false, nil
	}

	for _, device := range devices {
		if device.Entity.ID == nbID {
			return device.ID, true, nil
		}
	}

	return 0, false, nil
}

// Updates a service that has a firewallID annotation set.
// If an annotation is set, and the nodebalancer has a firewall that matches the ID, nothing to do
// If there's more than one firewall attached to the node-balancer, an error is returned as its not a supported use case.
// If there's only one firewall attached and it doesn't match what's in the annotation, the new firewall is attached and the old one removed
func (l *LinodeClient) updateServiceFirewall(ctx context.Context, service *v1.Service, nb *linodego.NodeBalancer) error {
	var newFirewallID int
	var err error

	// See if a firewall is attached to the nodebalancer first.
	firewalls, err := l.Client.ListNodeBalancerFirewalls(ctx, nb.ID, &linodego.ListOptions{})
	if err != nil {
		return err
	}
	if len(firewalls) > 1 {
		klog.Errorf("Found more than one firewall attached to nodebalancer: %d, firewall IDs: %v", nb.ID, firewalls)
		return ErrTooManyNBFirewalls
	}

	// get the ID of the firewall that is already attached to the nodeBalancer, if we have one.
	var existingFirewallID int
	if len(firewalls) == 1 {
		existingFirewallID = firewalls[0].ID
	}

	fwID := service.GetAnnotations()[annotations.AnnLinodeCloudFirewallID]
	newFirewallID, err = strconv.Atoi(fwID)
	if err != nil {
		return err
	}
	// if existing firewall and new firewall differs, attach the new firewall and remove the old.
	if existingFirewallID != newFirewallID {
		// attach new firewall.
		_, err = l.Client.CreateFirewallDevice(ctx, newFirewallID, linodego.FirewallDeviceCreateOptions{
			ID:   nb.ID,
			Type: "nodebalancer",
		})
		if err != nil {
			return err
		}
		// remove the existing firewall if it exists
		if existingFirewallID != 0 {
			deviceID, deviceExists, err := l.getNodeBalancerDeviceID(ctx, existingFirewallID, nb.ID)
			if err != nil {
				return err
			}

			if !deviceExists {
				return fmt.Errorf("error in fetching attached nodeBalancer device")
			}

			if err = l.Client.DeleteFirewallDevice(ctx, existingFirewallID, deviceID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (l *LinodeClient) updateNodeBalancerFirewallWithACL(
	ctx context.Context,
	loadBalancerName string,
	loadBalancerTags []string,
	service *v1.Service,
	nb *linodego.NodeBalancer,
) error {
	// See if a firewall is attached to the nodebalancer first.
	firewalls, err := l.Client.ListNodeBalancerFirewalls(ctx, nb.ID, &linodego.ListOptions{})
	if err != nil {
		return err
	}

	switch len(firewalls) {
	case 0:
		{
			// need to create a fw and attach it to our nb
			fwcreateOpts, err := CreateFirewallOptsForSvc(loadBalancerName, loadBalancerTags, service)
			if err != nil {
				return err
			}

			fw, err := l.Client.CreateFirewall(ctx, *fwcreateOpts)
			if err != nil {
				return err
			}
			// attach new firewall.
			if _, err = l.Client.CreateFirewallDevice(ctx, fw.ID, linodego.FirewallDeviceCreateOptions{
				ID:   nb.ID,
				Type: "nodebalancer",
			}); err != nil {
				return err
			}
		}
	case 1:
		{
			// We do not want to get into the complexity of reconciling differences, might as well just pull what's in the svc annotation now and update the fw.
			var acl aclConfig
			err := json.Unmarshal([]byte(service.GetAnnotations()[annotations.AnnLinodeCloudFirewallACL]), &acl)
			if err != nil {
				return err
			}

			changed := ruleChanged(firewalls[0].Rules, acl, service)
			if !changed {
				return nil
			}

			fwCreateOpts, err := CreateFirewallOptsForSvc(firewalls[0].Label, []string{""}, service)
			if err != nil {
				return err
			}
			if _, err = l.Client.UpdateFirewallRules(ctx, firewalls[0].ID, fwCreateOpts.Rules); err != nil {
				return err
			}
		}
	default:
		klog.Errorf("Found more than one firewall attached to nodebalancer: %d, firewall IDs: %v", nb.ID, firewalls)
		return ErrTooManyNBFirewalls
	}
	return nil
}

func CreateFirewallOptsForSvc(label string, tags []string, svc *v1.Service) (*linodego.FirewallCreateOptions, error) {
	// Fetch acl from annotation
	aclString := svc.GetAnnotations()[annotations.AnnLinodeCloudFirewallACL]
	fwcreateOpts := linodego.FirewallCreateOptions{
		Label: label,
		Tags:  tags,
	}
	servicePorts := make([]string, 0, len(svc.Spec.Ports))
	for _, port := range svc.Spec.Ports {
		servicePorts = append(servicePorts, strconv.Itoa(int(port.Port)))
	}

	portsString := strings.Join(servicePorts, ",")
	var acl aclConfig
	if err := json.Unmarshal([]byte(aclString), &acl); err != nil {
		return nil, err
	}
	// it is a problem if both are set, or if both are not set
	if (acl.AllowList != nil && acl.DenyList != nil) || (acl.AllowList == nil && acl.DenyList == nil) {
		return nil, ErrInvalidFWConfig
	}

	aclType := accept
	allowedIPs := acl.AllowList
	if acl.DenyList != nil {
		aclType = drop
		allowedIPs = acl.DenyList
	}

	if err := processACL(&fwcreateOpts, aclType, label, svc.Name, portsString, *allowedIPs); err != nil {
		return nil, err
	}
	return &fwcreateOpts, nil
}
