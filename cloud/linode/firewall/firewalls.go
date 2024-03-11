package firewall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/exp/slices"

	"github.com/linode/linodego"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/linode/linode-cloud-controller-manager/cloud/annotations"
	"github.com/linode/linode-cloud-controller-manager/cloud/linode/client"
)

const (
	maxFirewallRuleLabelLen = 32
	maxIPsPerFirewall       = 255
	maxRulesPerFirewall     = 25
)

var (
	ErrTooManyIPs           = errors.New("too many IPs in this ACL, will exceed rules per firewall limit")
	ErrTooManyNBFirewalls   = errors.New("too many firewalls attached to a nodebalancer")
	ErrTooManyNodeFirewalls = errors.New("too many firewalls attached to a node")
	ErrInvalidFWConfig      = errors.New("specify either an allowList or a denyList for a firewall")
)

type Firewalls struct {
	Client client.Client
}

func NewFirewalls(client client.Client) *Firewalls {
	return &Firewalls{Client: client}
}

type aclConfig struct {
	AllowList *linodego.NetworkAddresses `json:"allowList"`
	DenyList  *linodego.NetworkAddresses `json:"denyList"`
	Ports     []string                   `json:"ports"`
}

func (l *Firewalls) CreateFirewall(ctx context.Context, opts linodego.FirewallCreateOptions) (fw *linodego.Firewall, err error) {
	return l.Client.CreateFirewall(ctx, opts)
}

func (l *Firewalls) DeleteFirewall(ctx context.Context, firewall *linodego.Firewall) error {
	return l.Client.DeleteFirewall(ctx, firewall.ID)
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
		for _, ipv4 := range *ips.IPv4 {
			if !slices.Contains(ruleIPv4s, ipv4) {
				return true
			}
		}
	}

	if ips.IPv6 != nil {
		for _, ipv6 := range *ips.IPv6 {
			if !slices.Contains(ruleIPv6s, ipv6) {
				return true
			}
		}
	}

	return false
}

// ruleChanged takes an old FirewallRuleSet and new aclConfig and returns if
// the IPs of the FirewallRuleSet would be changed with the new ACL Config
func ruleChanged(old linodego.FirewallRuleSet, newACL aclConfig) bool {
	var ips *linodego.NetworkAddresses
	if newACL.AllowList != nil {
		// this is a allowList, this means that the rules should have `DROP` as inboundpolicy
		if old.InboundPolicy != "DROP" {
			return true
		}
		if (newACL.AllowList.IPv4 != nil || newACL.AllowList.IPv6 != nil) && len(old.Inbound) == 0 {
			return true
		}
		ips = newACL.AllowList
	}

	if newACL.DenyList != nil {
		if old.InboundPolicy != "ACCEPT" {
			return true
		}

		if (newACL.DenyList.IPv4 != nil || newACL.DenyList.IPv6 != nil) && len(old.Inbound) == 0 {
			return true
		}
		ips = newACL.DenyList
	}

	return ipsChanged(ips, old.Inbound)
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
			fwcreateOpts.Rules.Inbound = append(fwcreateOpts.Rules.Inbound, linodego.FirewallRule{
				Action:      aclType,
				Label:       ruleLabel,
				Description: fmt.Sprintf("Rule %d, Created by linode-ccm: %s, for %s", i, label, svcName),
				Protocol:    linodego.TCP, // Nodebalancers support only TCP.
				Ports:       ports,
				Addresses:   linodego.NetworkAddresses{IPv4: &v4chunk},
			})
		}

		ipv6chunks := chunkIPs(ipv6s)
		for i, chunk := range ipv6chunks {
			v6chunk := chunk
			fwcreateOpts.Rules.Inbound = append(fwcreateOpts.Rules.Inbound, linodego.FirewallRule{
				Action:      aclType,
				Label:       ruleLabel,
				Description: fmt.Sprintf("Rule %d, Created by linode-ccm: %s, for %s", i, label, svcName),
				Protocol:    linodego.TCP, // Nodebalancers support only TCP.
				Ports:       ports,
				Addresses:   linodego.NetworkAddresses{IPv6: &v6chunk},
			})
		}
	} else {
		fwcreateOpts.Rules.Inbound = append(fwcreateOpts.Rules.Inbound, linodego.FirewallRule{
			Action:      aclType,
			Label:       ruleLabel,
			Description: fmt.Sprintf("Created by linode-ccm: %s, for %s", label, svcName),
			Protocol:    linodego.TCP, // Nodebalancers support only TCP.
			Ports:       ports,
			Addresses:   ips,
		})
	}

	fwcreateOpts.Rules.OutboundPolicy = "ACCEPT"
	if aclType == "ACCEPT" {
		// if an allowlist is present, we drop everything else.
		fwcreateOpts.Rules.InboundPolicy = "DROP"
	} else {
		// if a denylist is present, we accept everything else.
		fwcreateOpts.Rules.InboundPolicy = "ACCEPT"
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
func (l *Firewalls) UpdateNodeBalancerFirewall(
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
		return fmt.Errorf("failed to list nodebalancer %d firewalls: %w", nb.ID, err)
	}

	if len(firewalls) == 0 {
		return nil
	}
	if len(firewalls) > 1 {
		klog.Errorf("Found more than one firewall attached to nodebalancer: %d, firewall IDs: %v", nb.ID, firewalls)
		return ErrTooManyNBFirewalls
	}

	return l.deleteFWDevice(ctx, firewalls[0].ID, nb.ID)
}

// UpdateNodeFirewall reconciles the firewall attached to the node
//
// This function does the following
//  1. If a firewallID annotation is present, it checks if the node has a firewall attached, and if it matches the annotationID
//     a. If the IDs match, nothing to do here.
//     b. If they don't match, the nb is attached to the new firewall and removed from the old one.
//  2. If a firewallACL annotation is present,
//     a. it checks if the node has a firewall attached, if a fw exists, it updates rules
//     b. if a fw does not exist, it creates one
//  3. If neither of these annotations are present,
//     a. AND if no firewalls are attached to the node, nothing to do.
//     b. if the node has ONE firewall attached, remove it from node, and clean up if nothing else is attached to it
//     c. If there are more than one firewall attached to it, then it's a problem, return an error
//  4. If both these annotations are present, the firewallID takes precedence, and the ACL annotation is ignored.
//
// If a user creates a firewall ID externally, and then switches to using a ACL, the CCM will take over the firewall that's attached to the node.
func (l *Firewalls) UpdateNodeFirewall(
	ctx context.Context,
	node *v1.Node,
	instance *linodego.Instance,
) error {
	// get the new firewall id from the annotation (if any).
	_, fwIDExists := node.GetAnnotations()[annotations.AnnLinodeNodeFirewallID]
	if fwIDExists { // If an ID exists, we ignore everything else and handle just that
		return l.updateNodeFirewall(ctx, node, instance)
	}

	// See if an ACL exists
	_, fwACLExists := node.GetAnnotations()[annotations.AnnLinodeNodeFirewallACL]
	if fwACLExists { // if an ACL exists, but no ID, just update the ACL on the fw.
		return l.updateNodeFirewallWithACL(ctx, node, instance)
	}

	// No firewall ID or ACL annotation, see if there are firewalls attached to our node
	firewalls, err := l.Client.ListInstanceFirewalls(ctx, instance.ID, &linodego.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list instance %d firewalls: %w", instance.ID, err)
	}

	if len(firewalls) == 0 {
		return nil
	}
	if len(firewalls) > 1 {
		klog.Errorf("Found more than one firewall attached to node: %d, firewall IDs: %v", instance.ID, firewalls)
		return ErrTooManyNodeFirewalls
	}

	return l.deleteFWDevice(ctx, firewalls[0].ID, instance.ID)
}

func (l *Firewalls) deleteFWDevice(ctx context.Context, firewallID int, deviceEntityID int) error {
	devices, err := l.Client.ListFirewallDevices(ctx, firewallID, &linodego.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list firewall devices: %w", err)
	}
	if len(devices) == 0 {
		// nothing to delete
		return nil
	}
	for _, device := range devices {
		if device.Entity.ID == deviceEntityID {
			if err = l.Client.DeleteFirewallDevice(ctx, firewallID, device.ID); err != nil {
				return fmt.Errorf(
					"failed to delete firewall %d device ID %d with enity %d: %w",
					firewallID,
					device.ID,
					deviceEntityID,
					err,
				)
			}
		}
	}
	// once we delete the device if found, we should see if there's anything attached to that firewall
	devices, err = l.Client.ListFirewallDevices(ctx, firewallID, &linodego.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to relist firewall %d devices: %w", firewallID, err)
	}
	if len(devices) == 0 {
		// nothing attached to it, clean it up
		if err = l.Client.DeleteFirewall(ctx, firewallID); err != nil {
			return fmt.Errorf("failed to delete firewall %d: %w", firewallID, err)
		}
	}
	// else let that firewall linger, don't mess with it.
	return nil
}

// Updates a service that has a firewallID annotation set.
// If an annotation is set, and the nodebalancer has a firewall that matches the ID, nothing to do
// If there's more than one firewall attached to the node-balancer, an error is returned as its not a supported use case.
// If there's only one firewall attached and it doesn't match what's in the annotation, the new firewall is attached and the old one removed
func (l *Firewalls) updateServiceFirewall(ctx context.Context, service *v1.Service, nb *linodego.NodeBalancer) error {
	var newFirewallID int
	var err error

	// See if a firewall is attached to the nodebalancer first.
	firewalls, err := l.Client.ListNodeBalancerFirewalls(ctx, nb.ID, &linodego.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list nodebalancer %d firewalls: %w", nb.ID, err)
	}
	if len(firewalls) > 1 {
		klog.Errorf("Found more than one firewall attached to nodebalancer: %d, firewall IDs: %v", nb.ID, firewalls)
		return ErrTooManyNBFirewalls
	}
	foundExistingFW := false
	if len(firewalls) == 1 {
		foundExistingFW = true
	}

	// get the ID of the firewall that is already attached to the nodeBalancer, if we have one.
	var existingFirewallID int
	if len(firewalls) == 1 {
		existingFirewallID = firewalls[0].ID
	}

	fwID := service.GetAnnotations()[annotations.AnnLinodeCloudFirewallID]
	newFirewallID, err = strconv.Atoi(fwID)
	if err != nil {
		return fmt.Errorf("failed to get firewall ID: %w", err)
	}
	// if existing firewall and new firewall differs, attach the new firewall and remove the old.
	if existingFirewallID != newFirewallID {
		// attach new firewall.
		_, err = l.Client.CreateFirewallDevice(ctx, newFirewallID, linodego.FirewallDeviceCreateOptions{
			ID:   nb.ID,
			Type: linodego.FirewallDeviceNodeBalancer,
		})
		if err != nil {
			return fmt.Errorf("failed to create firewall device: %w", err)
		}
		// remove the existing firewall if it exists
		if foundExistingFW {
			return l.deleteFWDevice(ctx, existingFirewallID, nb.ID)
		}
	}
	return nil
}

// Updates a node that has a firewallID annotation set.
// If an annotation is set, and the node has a firewall that matches the ID, nothing to do
// If there's more than one firewall attached to the node, an error is returned as its not a supported use case.
// If there's only one firewall attached and it doesn't match what's in the annotation, the new firewall is attached and the old one removed
func (l *Firewalls) updateNodeFirewall(ctx context.Context, node *v1.Node, instance *linodego.Instance) error {
	// See if a firewall is attached to the node first.
	firewalls, err := l.Client.ListInstanceFirewalls(ctx, instance.ID, &linodego.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list instance %d firewalls: %w", instance.ID, err)
	}
	if len(firewalls) > 1 {
		klog.Errorf("Found more than one firewall attached to node: %d, firewall IDs: %v", instance.ID, firewalls)
		return ErrTooManyNodeFirewalls
	}
	foundExistingFW := false
	if len(firewalls) == 1 {
		foundExistingFW = true
	}

	// get the ID of the firewall that is already attached to the node, if we have one.
	var existingFirewallID int
	if len(firewalls) == 1 {
		existingFirewallID = firewalls[0].ID
	}

	annFirewallID, err := strconv.Atoi(node.GetAnnotations()[annotations.AnnLinodeNodeFirewallID])
	if err != nil {
		return fmt.Errorf("failed to get firewall ID: %w", err)
	}
	// if existing firewall and new firewall differs, attach the new firewall and remove the old.
	if existingFirewallID != annFirewallID {
		// attach new firewall.
		_, err = l.Client.CreateFirewallDevice(ctx, annFirewallID, linodego.FirewallDeviceCreateOptions{
			ID:   instance.ID,
			Type: linodego.FirewallDeviceLinode,
		})
		if err != nil {
			return fmt.Errorf("failed to create firewall device: %w", err)
		}
		// remove the existing firewall if it exists
		if foundExistingFW {
			return l.deleteFWDevice(ctx, existingFirewallID, instance.ID)
		}
	}
	return nil
}

func (l *Firewalls) updateNodeBalancerFirewallWithACL(
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
				return fmt.Errorf("failed to create firewall options: %w", err)
			}

			fw, err := l.Client.CreateFirewall(ctx, *fwcreateOpts)
			if err != nil {
				return fmt.Errorf("failed to create firewall: %w", err)
			}
			// attach new firewall.
			if _, err = l.Client.CreateFirewallDevice(ctx, fw.ID, linodego.FirewallDeviceCreateOptions{
				ID:   nb.ID,
				Type: linodego.FirewallDeviceNodeBalancer,
			}); err != nil {
				return fmt.Errorf("failed to create firewall device: %w", err)
			}
		}
	case 1:
		{
			// We do not want to get into the complexity of reconciling differences, might as well just pull what's in the svc annotation now and update the fw.
			var acl aclConfig
			err := json.Unmarshal([]byte(service.GetAnnotations()[annotations.AnnLinodeCloudFirewallACL]), &acl)
			if err != nil {
				return fmt.Errorf("failed unmarshal ACL Config: %w", err)
			}

			changed := ruleChanged(firewalls[0].Rules, acl)
			if !changed {
				return nil
			}

			fwCreateOpts, err := CreateFirewallOptsForSvc(service.Name, []string{""}, service)
			if err != nil {
				return fmt.Errorf("failed to create firewall options: %w", err)
			}
			if _, err = l.Client.UpdateFirewallRules(ctx, firewalls[0].ID, fwCreateOpts.Rules); err != nil {
				return fmt.Errorf("failed to update firewall rules: %w", err)
			}
		}
	default:
		klog.Errorf("Found more than one firewall attached to nodebalancer: %d, firewall IDs: %v", nb.ID, firewalls)
		return ErrTooManyNBFirewalls
	}
	return nil
}

func (l *Firewalls) updateNodeFirewallWithACL(
	ctx context.Context,
	node *v1.Node,
	instance *linodego.Instance,
) error {
	// See if a firewall is attached to the node first.
	firewalls, err := l.Client.ListInstanceFirewalls(ctx, instance.ID, &linodego.ListOptions{})
	if err != nil {
		return err
	}

	switch len(firewalls) {
	case 0:
		{
			// need to create a fw and attach it to our node
			fwcreateOpts, err := CreateFirewallOptsForNode(instance, node)
			if err != nil {
				return fmt.Errorf("failed to create firewall options: %w", err)
			}

			fw, err := l.Client.CreateFirewall(ctx, *fwcreateOpts)
			if err != nil {
				return fmt.Errorf("failed to create firewall: %w", err)
			}
			// attach new firewall.
			if _, err = l.Client.CreateFirewallDevice(ctx, fw.ID, linodego.FirewallDeviceCreateOptions{
				ID:   instance.ID,
				Type: linodego.FirewallDeviceLinode,
			}); err != nil {
				return fmt.Errorf("failed to create firewall device: %w", err)
			}
		}
	case 1:
		{
			// We do not want to get into the complexity of reconciling differences, might as well just pull what's in the node annotation now and update the fw.
			var acl aclConfig
			if err := json.Unmarshal([]byte(node.GetAnnotations()[annotations.AnnLinodeNodeFirewallACL]), &acl); err != nil {
				return fmt.Errorf("failed unmarshal ACL Config: %w", err)
			}

			changed := ruleChanged(firewalls[0].Rules, acl)
			if !changed {
				return nil
			}

			fwCreateOpts, err := CreateFirewallOptsForNode(instance, node)
			if err != nil {
				return fmt.Errorf("failed to create firewall options: %w", err)
			}
			if _, err = l.Client.UpdateFirewallRules(ctx, firewalls[0].ID, fwCreateOpts.Rules); err != nil {
				return fmt.Errorf("failed to update firewall rules: %w", err)
			}
		}
	default:
		klog.Errorf("Found more than one firewall attached to node: %d, firewall IDs: %v", instance.ID, firewalls)
		return ErrTooManyNodeFirewalls
	}
	return nil
}

// CreateFirewallOptsForSvc creates a FirewallCreateOptions based on ACL annotations on a Service
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
	portsString := strings.Join(servicePorts[:], ",")

	var acl aclConfig
	if err := json.Unmarshal([]byte(aclString), &acl); err != nil {
		return nil, err
	}
	// it is a problem if both are set, or if both are not set
	if (acl.AllowList != nil && acl.DenyList != nil) || (acl.AllowList == nil && acl.DenyList == nil) {
		return nil, ErrInvalidFWConfig
	}

	aclType := "ACCEPT"
	allowedIPs := acl.AllowList
	if acl.DenyList != nil {
		aclType = "DROP"
		allowedIPs = acl.DenyList
	}

	if err := processACL(&fwcreateOpts, aclType, label, svc.Name, portsString, *allowedIPs); err != nil {
		return nil, err
	}
	return &fwcreateOpts, nil
}

// CreateFirewallOptsForNode creates a FirewallCreateOptions based on ACL annotations on a Node
func CreateFirewallOptsForNode(instance *linodego.Instance, node *v1.Node) (*linodego.FirewallCreateOptions, error) {
	// Fetch acl from annotation
	aclString := node.GetAnnotations()[annotations.AnnLinodeNodeFirewallACL]
	fwcreateOpts := linodego.FirewallCreateOptions{
		Label: node.Name,
		Tags:  instance.Tags,
	}

	var acl aclConfig
	if err := json.Unmarshal([]byte(aclString), &acl); err != nil {
		return nil, err
	}
	// it is a problem if both are set, or if both are not set
	if (acl.AllowList != nil && acl.DenyList != nil) || (acl.AllowList == nil && acl.DenyList == nil) {
		return nil, ErrInvalidFWConfig
	}

	aclType := "ACCEPT"
	allowedIPs := acl.AllowList
	if acl.DenyList != nil {
		aclType = "DROP"
		allowedIPs = acl.DenyList
	}

	portsString := strings.Join(acl.Ports[:], ",")
	if err := processACL(&fwcreateOpts, aclType, node.Name, node.Name, portsString, *allowedIPs); err != nil {
		return nil, err
	}
	return &fwcreateOpts, nil
}
