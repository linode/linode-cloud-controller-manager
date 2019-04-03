package e2e_test

import (
	"fmt"
	"github.com/linode/linode-cloud-controller-manager/test/e2e/framework"
	"github.com/linode/linode-cloud-controller-manager/test/test-server/client"
	"net/http"
	"strings"

	//"github.com/linode/linode-cloud-controller-manager/test/test-server/client"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CloudControllerManager", func() {
	var (
		err error
		f *framework.Invocation
		workers []string

	)
	//pods = []string{"test-pod-1", "test-pod-2"}

	BeforeEach(func() {
		f = root.Invoke()
		workers, err = f.GetNodeList()
		Expect(err).NotTo(HaveOccurred())
		Expect(len(workers)).Should(BeNumerically(">=", 2))

	})

	var createPodWithLabel = func(pods []string, labels map[string]string) {
		for i, pod := range pods {
			p:= f.LoadBalancer.GetPodObject(pod, workers[i], labels)
			err = f.LoadBalancer.CreatePod(p)
			Expect(err).NotTo(HaveOccurred())
		}
	}

	var createServiceWithSelector = func(selector map[string]string) {
		err = f.LoadBalancer.CreateService(selector)
		Expect(err).NotTo(HaveOccurred())
	}

	Describe("Test", func() {
		Context("Simple", func() {
			Context("Load Balancer", func() {
				var (
					pods []string
					labels map[string]string
				)

				BeforeEach(func() {
					pods = []string{"test-pod-1", "test-pod-2"}
					labels = map[string]string{
						"app": "test-loadbalancer",
					}
					createPodWithLabel(pods, labels)
					createServiceWithSelector(labels)
				})


				It("should reach all pods", func() {
					By("Checking tcp response")
					eps, err := f.LoadBalancer.GetHTTPEndpoints()
					Expect(err).NotTo(HaveOccurred())
					Expect(len(eps)).Should(BeNumerically(">=", 1))

					var counter1, counter2 int
					for i:=1; i<=100; i++ {
						err = f.LoadBalancer.DoHTTP(framework.MaxRetry, "", eps, "GET", "", func(resp *client.Response) bool {
							if strings.Contains(resp.Body, pods[0]) {
								counter1++
							}else if strings.Contains(resp.Body, pods[1]) {
									counter2++
							}

							return Expect(resp.Status).Should(Equal(http.StatusOK))
						})
					}
					fmt.Println(counter1, "<>", counter2)
					Expect(counter1).Should(BeNumerically(">", 0))
					Expect(counter2).Should(BeNumerically(">", 0))

					/*err = f.LoadBalancer.DoTCP(framework.MaxRetry, eps, func(resp *client.Response) bool {
						return r
					} )*/

				})

			})
		})
	})

})
