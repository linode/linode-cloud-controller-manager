package linode

const (
	// annLinodeDefaultProtocol is the annotation used to specify the default protocol
	// for Linode load balancers. Options are tcp, http and https. Defaults to tcp.
	annLinodeDefaultProtocol      = "service.beta.kubernetes.io/linode-loadbalancer-default-protocol"
	annLinodePortConfigPrefix     = "service.beta.kubernetes.io/linode-loadbalancer-port-"
	annLinodeDefaultProxyProtocol = "service.beta.kubernetes.io/linode-loadbalancer-default-proxy-protocol"

	annLinodeCheckPath       = "service.beta.kubernetes.io/linode-loadbalancer-check-path"
	annLinodeCheckBody       = "service.beta.kubernetes.io/linode-loadbalancer-check-body"
	annLinodeHealthCheckType = "service.beta.kubernetes.io/linode-loadbalancer-check-type"

	annLinodeHealthCheckInterval = "service.beta.kubernetes.io/linode-loadbalancer-check-interval"
	annLinodeHealthCheckTimeout  = "service.beta.kubernetes.io/linode-loadbalancer-check-timeout"
	annLinodeHealthCheckAttempts = "service.beta.kubernetes.io/linode-loadbalancer-check-attempts"
	annLinodeHealthCheckPassive  = "service.beta.kubernetes.io/linode-loadbalancer-check-passive"

	// annLinodeThrottle is the annotation specifying the value of the Client Connection
	// Throttle, which limits the number of subsequent new connections per second from the
	// same client IP. Options are a number between 1-20, or 0 to disable. Defaults to 20.
	annLinodeThrottle = "service.beta.kubernetes.io/linode-loadbalancer-throttle"

	annLinodeLoadBalancerPreserve = "service.beta.kubernetes.io/linode-loadbalancer-preserve"
	annLinodeNodeBalancerID       = "service.beta.kubernetes.io/linode-loadbalancer-nodebalancer-id"

	annLinodeHostnameOnlyIngress = "service.beta.kubernetes.io/linode-loadbalancer-hostname-only-ingress"
	annLinodeLoadBalancerTags    = "service.beta.kubernetes.io/linode-loadbalancer-tags"
	annLinodeCloudFirewallID     = "service.beta.kubernetes.io/linode-loadbalancer-firewall-id"

	annLinodeNodePrivateIP = "node.k8s.linode.com/private-ip"
	annLinodeUUID          = "node.k8s.linode.com/uuid"
)
