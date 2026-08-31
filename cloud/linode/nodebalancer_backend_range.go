package linode

import (
	"encoding/binary"
	"fmt"
	"net/netip"

	"github.com/linode/linodego/v2"
)

const nodeBalancerBackendRangePrefix = 30

func allocateNodeBalancerBackendIPv4Range(backendCIDR, reservedCIDR string, subnet *linodego.VPCSubnet) (string, error) {
	if subnet == nil {
		return "", fmt.Errorf("cannot allocate NodeBalancer backend range from a nil VPC subnet")
	}

	if err := validateNodeBalancerBackendIPv4Reservation(backendCIDR, reservedCIDR); err != nil {
		return "", err
	}

	backend, err := parseIPv4Prefix(backendCIDR)
	if err != nil {
		return "", fmt.Errorf("invalid NodeBalancer backend CIDR: %w", err)
	}
	reserved, err := parseIPv4Prefix(reservedCIDR)
	if err != nil {
		return "", fmt.Errorf("invalid reserved NodeBalancer backend range: %w", err)
	}

	subnetPrefix, err := parseIPv4Prefix(subnet.IPv4)
	if err != nil {
		return "", fmt.Errorf("invalid VPC subnet CIDR: %w", err)
	}
	if !prefixContains(subnetPrefix, backend) {
		return "", fmt.Errorf("NodeBalancer backend CIDR %s is not within VPC subnet %s", backend, subnetPrefix)
	}

	rangeCount := uint64(1) << uint(nodeBalancerBackendRangePrefix-backend.Bits())

	assigned := make([]netip.Prefix, 0, len(subnet.Nodebalancers))
	for _, nodeBalancer := range subnet.Nodebalancers {
		if nodeBalancer.Ipv4Range == "" {
			continue
		}
		assignedRange, err := parseIPv4Prefix(nodeBalancer.Ipv4Range)
		if err != nil {
			return "", fmt.Errorf("invalid IPv4 range %q on NodeBalancer %d: %w", nodeBalancer.Ipv4Range, nodeBalancer.ID, err)
		}
		if prefixesOverlap(assignedRange, reserved) {
			return "", fmt.Errorf("reserved NodeBalancer backend range %s is already assigned to NodeBalancer %d", reserved, nodeBalancer.ID)
		}
		if prefixesOverlap(assignedRange, backend) {
			assigned = append(assigned, assignedRange)
		}
	}

	for index := uint64(0); index < rangeCount-1; index++ {
		candidate, err := ipv4SubprefixAt(backend, nodeBalancerBackendRangePrefix, index)
		if err != nil {
			return "", err
		}
		if !overlapsAnyPrefix(candidate, assigned) {
			return candidate.String(), nil
		}
	}

	return "", fmt.Errorf("NodeBalancer backend CIDR %s has no available /30 ranges", backend)
}

func validateNodeBalancerBackendIPv4Reservation(backendCIDR, reservedCIDR string) error {
	if backendCIDR == "" {
		return fmt.Errorf("NodeBalancer backend IPv4 subnet is required when a reserved range is configured")
	}

	reserved, err := parseIPv4Prefix(reservedCIDR)
	if err != nil {
		return fmt.Errorf("invalid reserved NodeBalancer backend range: %w", err)
	}
	if reserved.Bits() != nodeBalancerBackendRangePrefix {
		return fmt.Errorf("reserved NodeBalancer backend range %s must be a /30", reserved)
	}

	expected, err := highestNodeBalancerBackendIPv4Range(backendCIDR)
	if err != nil {
		return fmt.Errorf("invalid NodeBalancer backend subnet: %w", err)
	}
	if reserved != expected {
		return fmt.Errorf("reserved NodeBalancer backend range %s must be the highest /30 in %s (%s)", reserved, backendCIDR, expected)
	}
	return nil
}

func highestNodeBalancerBackendIPv4Range(backendCIDR string) (netip.Prefix, error) {
	backend, err := parseIPv4Prefix(backendCIDR)
	if err != nil {
		return netip.Prefix{}, err
	}
	if backend.Bits() >= nodeBalancerBackendRangePrefix {
		return netip.Prefix{}, fmt.Errorf("NodeBalancer backend CIDR %s has no allocable /30 after reserving its highest /30", backend)
	}

	rangeCount := uint64(1) << uint(nodeBalancerBackendRangePrefix-backend.Bits())
	return ipv4SubprefixAt(backend, nodeBalancerBackendRangePrefix, rangeCount-1)
}

func parseIPv4Prefix(value string) (netip.Prefix, error) {
	prefix, err := netip.ParsePrefix(value)
	if err != nil {
		return netip.Prefix{}, err
	}
	if !prefix.Addr().Is4() {
		return netip.Prefix{}, fmt.Errorf("%s is not an IPv4 prefix", value)
	}
	return prefix.Masked(), nil
}

func ipv4SubprefixAt(parent netip.Prefix, childBits int, index uint64) (netip.Prefix, error) {
	if !parent.Addr().Is4() || childBits < parent.Bits() || childBits > 32 {
		return netip.Prefix{}, fmt.Errorf("cannot select /%d from prefix %s", childBits, parent)
	}

	childCount := uint64(1) << uint(childBits-parent.Bits())
	if index >= childCount {
		return netip.Prefix{}, fmt.Errorf("subprefix index %d is outside prefix %s", index, parent)
	}

	baseBytes := parent.Masked().Addr().As4()
	base := uint64(binary.BigEndian.Uint32(baseBytes[:]))
	blockSize := uint64(1) << uint(32-childBits)
	address := base + index*blockSize
	var addressBytes [4]byte
	binary.BigEndian.PutUint32(addressBytes[:], uint32(address))

	return netip.PrefixFrom(netip.AddrFrom4(addressBytes), childBits), nil
}

func prefixContains(outer, inner netip.Prefix) bool {
	return outer.Bits() <= inner.Bits() && outer.Contains(inner.Addr())
}

func prefixesOverlap(first, second netip.Prefix) bool {
	return first.Contains(second.Addr()) || second.Contains(first.Addr())
}

func overlapsAnyPrefix(candidate netip.Prefix, prefixes []netip.Prefix) bool {
	for _, prefix := range prefixes {
		if prefixesOverlap(candidate, prefix) {
			return true
		}
	}
	return false
}
