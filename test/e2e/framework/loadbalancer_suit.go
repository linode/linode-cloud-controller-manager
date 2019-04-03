package framework

import (
	"errors"
	"fmt"
	"github.com/golang/glog"
	"github.com/linode/linode-cloud-controller-manager/test/test-server/client"
)

/*func (i *lbInvocation) Setup() error  {
	if err := i.setupTestServers(); err != nil {
		return err
	}
	return i.waitForTestServer()
}
*/
func (i *lbInvocation) GetHTTPEndpoints() ([]string, error) {
		return i.getLoadBalancerURLs()
}


func (i *lbInvocation) DoHTTP(retryCount int, host string, eps []string, method, path string, matcher func(resp *client.Response) bool) error {
	for _, url := range eps {
		fmt.Println(url)
		resp, err := client.NewTestHTTPClient(url).WithHost(host).Method(method).Path(path).DoWithRetry(retryCount)
		if err != nil {
			return err
		}

		glog.Infoln("HTTP Response received from server", *resp)
		if !matcher(resp) {
			return errors.New("Failed to match")
		}
	}
	return nil
}
