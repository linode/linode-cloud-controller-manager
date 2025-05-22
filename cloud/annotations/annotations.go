package annotations

const (
	// AnnLinodeDefaultProtocol is the annotation used to specify the default protocol
	// for Linode load balancers. Options are tcp, http and https. Defaults to tcp.
	AnnLinodeDefaultProtocol      = "service.beta.kubernetes.io/linode-loadbalancer-default-protocol"
	AnnLinodePortConfigPrefix     = "service.beta.kubernetes.io/linode-loadbalancer-port-"
	AnnLinodeDefaultProxyProtocol = "service.beta.kubernetes.io/linode-loadbalancer-default-proxy-protocol"
	AnnLinodeDefaultAlgorithm     = "service.beta.kubernetes.io/linode-loadbalancer-default-algorithm"
	AnnLinodeDefaultStickiness    = "service.beta.kubernetes.io/linode-loadbalancer-default-stickiness"

	AnnLinodeCheckPath       = "service.beta.kubernetes.io/linode-loadbalancer-check-path"
	AnnLinodeCheckBody       = "service.beta.kubernetes.io/linode-loadbalancer-check-body"
	AnnLinodeHealthCheckType = "service.beta.kubernetes.io/linode-loadbalancer-check-type"

	AnnLinodeHealthCheckInterval = "service.beta.kubernetes.io/linode-loadbalancer-check-interval"
	AnnLinodeHealthCheckTimeout  = "service.beta.kubernetes.io/linode-loadbalancer-check-timeout"
	AnnLinodeHealthCheckAttempts = "service.beta.kubernetes.io/linode-loadbalancer-check-attempts"
	AnnLinodeHealthCheckPassive  = "service.beta.kubernetes.io/linode-loadbalancer-check-passive"

	AnnLinodeUDPCheckPort = "service.beta.kubernetes.io/linode-loadbalancer-udp-check-port"

	// AnnLinodeThrottle is the annotation specifying the value of the Client Connection
	// Throttle, which limits the number of subsequent new connections per second from the
	// same client IP. Options are a number between 1-20, or 0 to disable. Defaults to 20.
	AnnLinodeThrottle = "service.beta.kubernetes.io/linode-loadbalancer-throttle"

	AnnLinodeLoadBalancerPreserve = "service.beta.kubernetes.io/linode-loadbalancer-preserve"
	AnnLinodeNodeBalancerID       = "service.beta.kubernetes.io/linode-loadbalancer-nodebalancer-id"
	AnnLinodeNodeBalancerType     = "service.beta.kubernetes.io/linode-loadbalancer-nodebalancer-type"

	AnnLinodeHostnameOnlyIngress = "service.beta.kubernetes.io/linode-loadbalancer-hostname-only-ingress"
	AnnLinodeLoadBalancerTags    = "service.beta.kubernetes.io/linode-loadbalancer-tags"
	AnnLinodeCloudFirewallID     = "service.beta.kubernetes.io/linode-loadbalancer-firewall-id"
	AnnLinodeCloudFirewallACL    = "service.beta.kubernetes.io/linode-loadbalancer-firewall-acl"

	// AnnLinodeEnableIPv6Ingress is the annotation used to specify that a service should include both IPv4 and IPv6
	// addresses for its LoadBalancer ingress. When set to "true", both addresses will be included in the status.
	AnnLinodeEnableIPv6Ingress = "service.beta.kubernetes.io/linode-loadbalancer-enable-ipv6-ingress"

	AnnLinodeNodePrivateIP = "node.k8s.linode.com/private-ip"
	AnnLinodeHostUUID      = "node.k8s.linode.com/host-uuid"

	AnnLinodeNodeIPSharingUpdated = "node.k8s.linode.com/ip-sharing-updated"

	NodeBalancerBackendIPv4Range = "service.beta.kubernetes.io/linode-loadbalancer-backend-ipv4-range"

	NodeBalancerBackendVPCName    = "service.beta.kubernetes.io/linode-loadbalancer-backend-vpc-name"
	NodeBalancerBackendSubnetName = "service.beta.kubernetes.io/linode-loadbalancer-backend-subnet-name"
	NodeBalancerBackendSubnetID   = "service.beta.kubernetes.io/linode-loadbalancer-backend-subnet-id"
)
