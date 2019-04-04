package framework

func (i *lbInvocation) GetHTTPEndpoints() ([]string, error) {
	return i.getLoadBalancerURLs()
}
