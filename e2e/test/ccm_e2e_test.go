package test

import (
	"e2e_test/test/framework"
	"fmt"
	"github.com/appscode/go/wait"
	"github.com/codeskyblue/go-sh"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"net/url"
	"strings"
)

var _ = Describe("CloudControllerManager", func() {
	var (
		err     error
		f       *framework.Invocation
		workers []string
	)

	const (
		annLinodeLoadBalancerTLS = "service.beta.kubernetes.io/linode-loadbalancer-tls"
		annLinodeProtocol        = "service.beta.kubernetes.io/linode-loadbalancer-protocol"
	)

	BeforeEach(func() {
		f = root.Invoke()
		workers, err = f.GetNodeList()
		Expect(err).NotTo(HaveOccurred())
		Expect(len(workers)).Should(BeNumerically(">=", 2))
	})

	var createPodWithLabel = func(pods []string, labels map[string]string) {
		for i, pod := range pods {
			p := f.LoadBalancer.GetPodObject(pod, workers[i], labels)
			err = f.LoadBalancer.CreatePod(p)
			Expect(err).NotTo(HaveOccurred())
		}
	}

	var deletePods = func(pods []string) {
		for _, pod := range pods {
			err = f.LoadBalancer.DeletePod(pod)
			Expect(err).NotTo(HaveOccurred())
		}
	}

	var deleteService = func() {
		err = f.LoadBalancer.DeleteService()
		Expect(err).NotTo(HaveOccurred())
	}

	var deleteSecret = func() {
		err = f.LoadBalancer.DeleteSecret()
		Expect(err).NotTo(HaveOccurred())
	}

	var createServiceWithSelector = func(selector map[string]string) {
		err = f.LoadBalancer.CreateService(selector, nil)
		Expect(err).NotTo(HaveOccurred())
	}

	var createServiceWithAnnotations = func(labels map[string]string, annotations map[string]string) {
		err = f.LoadBalancer.CreateService(labels, annotations)
		Expect(err).NotTo(HaveOccurred())
	}

	Describe("Test", func() {
		Context("Simple", func() {
			Context("Load Balancer", func() {
				var (
					pods   []string
					labels map[string]string
				)

				BeforeEach(func() {
					pods = []string{"test-pod-1", "test-pod-2"}
					labels = map[string]string{
						"app": "test-loadbalancer",
					}

					By("Creating Pods")
					createPodWithLabel(pods, labels)

					By("Creating Service")
					createServiceWithSelector(labels)
				})

				AfterEach(func() {
					By("Deleting the Pods")
					deletePods(pods)

					By("Deleting the Service")
					deleteService()
				})

				It("should reach all pods", func() {
					By("Checking TCP Response")
					eps, err := f.LoadBalancer.GetHTTPEndpoints()
					Expect(err).NotTo(HaveOccurred())
					Expect(len(eps)).Should(BeNumerically(">=", 1))

					var counter1, counter2 int

					By("Waiting for Response from the LoadBalancer url: " + eps[0])
					err = wait.PollImmediate(framework.RetryInterval, framework.RetryTimout, func() (bool, error) {
						resp, err := sh.Command("curl", "--max-time", "5", "-s", eps[0]).Output()
						if err != nil {
							return false, nil
						}
						stringResp := string(resp)
						if strings.Contains(stringResp, pods[0]) {
							fmt.Println("Got response from " + pods[0])
							counter1++
						} else if strings.Contains(stringResp, pods[1]) {
							fmt.Println("Got response from " + pods[1])
							counter2++
						}

						if counter1 > 0 && counter2 > 0 {
							return true, nil
						}
						return false, nil
					})
					Expect(counter1).Should(BeNumerically(">", 0))
					Expect(counter2).Should(BeNumerically(">", 0))
				})
			})
		})
	})

	Describe("Test", func() {
		Context("LoadBalancer", func() {
			Context("With single TLS port", func() {
				var (
					pods        []string
					labels      map[string]string
					annotations map[string]string
					secretName  string
				)
				BeforeEach(func() {
					pods = []string{"test-pod"}
					secretName = "tls-secret"
					labels = map[string]string{
						"app": "test-loadbalancer",
					}
					annotations = map[string]string{
						annLinodeLoadBalancerTLS: `[ { "tls-secret-name": "` + secretName + `", "port": 80} ]`,
						annLinodeProtocol:        "https",
					}

					By("Creating Pod")
					createPodWithLabel(pods, labels)

					By("Creating Secret")
					err = f.LoadBalancer.CreateTLSSecret("tls-secret", "linode.test")
					Expect(err).NotTo(HaveOccurred())

					By("Creating Service")
					createServiceWithAnnotations(labels, annotations)
				})

				AfterEach(func() {
					By("Deleting the Secrets")
					deletePods(pods)

					By("Deleting the Service")
					deleteService()

					By("Deleting the Secret")
					deleteSecret()
				})

				It("should reach the pod via tls", func() {
					By("Checking TCP Response")
					eps, err := f.LoadBalancer.GetHTTPEndpoints()
					Expect(err).NotTo(HaveOccurred())
					Expect(len(eps)).Should(BeNumerically(">=", 1))

					var counter int

					By("Waiting for Response from the LoadBalancer url: " + eps[0])
					err = wait.PollImmediate(framework.RetryInterval, framework.RetryTimout, func() (bool, error) {
						u, err := url.Parse(eps[0])
						if err != nil {
							return false, nil
						}
						ipPort := strings.Split(u.Host, ":")

						session := sh.NewSession()
						session.ShowCMD = true
						resp, err := session.Command("curl", "--max-time", "5", "--resolve", "linode.test:"+ipPort[1]+":"+ipPort[0]+"", "--cacert", "certificates/ca.crt", "https://linode.test:80", "-s").Output()
						stringResp := string(resp)
						if err != nil {
							return false, nil
						}

						if strings.Contains(stringResp, pods[0]) {
							fmt.Println("Got response from " + pods[0] + " using url " + eps[0])
							counter++
						}

						if counter > 0 {
							return true, nil
						}
						return false, nil
					})
					Expect(counter).Should(BeNumerically(">", 0))
				})
			})
		})
	})

})
