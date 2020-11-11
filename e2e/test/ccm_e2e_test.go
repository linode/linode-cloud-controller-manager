package test

import (
	"context"
	"e2e_test/test/framework"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/appscode/go/wait"
	"github.com/codeskyblue/go-sh"
	"github.com/linode/linodego"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("e2e tests", func() {
	var (
		err     error
		f       *framework.Invocation
		workers []string
	)

	const (
		annLinodeProxyProtocol        = "service.beta.kubernetes.io/linode-loadbalancer-proxy-protocol"
		annLinodeDefaultProtocol      = "service.beta.kubernetes.io/linode-loadbalancer-default-protocol"
		annLinodePortConfigPrefix     = "service.beta.kubernetes.io/linode-loadbalancer-port-"
		annLinodeLoadBalancerPreserve = "service.beta.kubernetes.io/linode-loadbalancer-preserve"
		annLinodeHealthCheckType      = "service.beta.kubernetes.io/linode-loadbalancer-check-type"
		annLinodeCheckBody            = "service.beta.kubernetes.io/linode-loadbalancer-check-body"
		annLinodeCheckPath            = "service.beta.kubernetes.io/linode-loadbalancer-check-path"
		annLinodeHealthCheckInterval  = "service.beta.kubernetes.io/linode-loadbalancer-check-interval"
		annLinodeHealthCheckTimeout   = "service.beta.kubernetes.io/linode-loadbalancer-check-timeout"
		annLinodeHealthCheckAttempts  = "service.beta.kubernetes.io/linode-loadbalancer-check-attempts"
		annLinodeHealthCheckPassive   = "service.beta.kubernetes.io/linode-loadbalancer-check-passive"
		annLinodeNodeBalancerID       = "service.beta.kubernetes.io/linode-loadbalancer-nodebalancer-id"
	)

	BeforeEach(func() {
		f = root.Invoke()
		workers, err = f.GetNodeList()
		Expect(err).NotTo(HaveOccurred())
		Expect(len(workers)).Should(BeNumerically(">=", 2))
	})

	var createPodWithLabel = func(pods []string, ports []core.ContainerPort, image string, labels map[string]string, selectNode bool) {
		for i, pod := range pods {
			p := f.LoadBalancer.GetPodObject(pod, image, ports, labels)
			if selectNode {
				p = f.LoadBalancer.SetNodeSelector(p, workers[i])
			}
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

	var deleteSecret = func(name string) {
		err = f.LoadBalancer.DeleteSecret(name)
		Expect(err).NotTo(HaveOccurred())
	}

	var createServiceWithSelector = func(selector map[string]string, ports []core.ServicePort, isSessionAffinityClientIP bool) {
		err = f.LoadBalancer.CreateService(selector, nil, ports, isSessionAffinityClientIP)
		Expect(err).NotTo(HaveOccurred())
	}

	var createServiceWithAnnotations = func(labels map[string]string, annotations map[string]string, ports []core.ServicePort, isSessionAffinityClientIP bool) {
		err = f.LoadBalancer.CreateService(labels, annotations, ports, isSessionAffinityClientIP)
		Expect(err).NotTo(HaveOccurred())
	}

	var updateServiceWithAnnotations = func(labels map[string]string, annotations map[string]string, ports []core.ServicePort, isSessionAffinityClientIP bool) {
		err = f.LoadBalancer.UpdateService(labels, annotations, ports, isSessionAffinityClientIP)
		Expect(err).NotTo(HaveOccurred())
	}

	var deleteNodeBalancer = func(id int) {
		err = getLinodeClient().DeleteNodeBalancer(context.Background(), id)
		Expect(err).NotTo(HaveOccurred())
	}

	var createNodeBalancer = func() int {
		var nb *linodego.NodeBalancer
		nb, err = getLinodeClient().CreateNodeBalancer(context.Background(), linodego.NodeBalancerCreateOptions{
			Region: "eu-west",
		})
		Expect(err).NotTo(HaveOccurred())
		return nb.ID
	}

	var getResponseFromSamePod = func(link string) {
		var oldResp, newResp string
		Eventually(func() string {
			resp, err := http.Get(link)
			if err == nil {
				byteData, _ := ioutil.ReadAll(resp.Body)
				defer resp.Body.Close()
				oldResp = string(byteData)
			}

			return oldResp
		}).ShouldNot(Equal(""))

		for i := 0; i <= 10; i++ {
			resp, err := http.Get(link)
			if err == nil {
				byteData, _ := ioutil.ReadAll(resp.Body)
				defer resp.Body.Close()
				newResp = string(byteData)
				log.Println(newResp)
			}
		}
	}

	var checkNumberOfWorkerNodes = func(numNodes int) {
		Eventually(func() int {
			workers, err = f.GetNodeList()
			Expect(err).NotTo(HaveOccurred())
			return len(workers)
		}).Should(Equal(numNodes))
	}

	var checkNumberOfUpNodes = func(numNodes int) {
		By("Checking the Number of Up Nodes")
		Eventually(func() int {
			nbConfig, err := f.LoadBalancer.GetNodeBalancerConfig(framework.TestServerResourceName)
			Expect(err).NotTo(HaveOccurred())
			return nbConfig.NodesStatus.Up
		}).Should(Equal(numNodes))
	}

	var checkNodeBalancerExists = func(id int) {
		By("Checking if the NodeBalancer exists")
		nb, err := getLinodeClient().GetNodeBalancer(context.Background(), id)
		Expect(err).NotTo(HaveOccurred())
		Expect(nb.ID).Should(Equal(nb.ID))
	}

	var checkNodeBalancerNotExists = func(id int) {
		nb, err := getLinodeClient().GetNodeBalancer(context.Background(), id)
		Expect(nb).To(BeNil())
		Expect(err).ToNot(BeNil())

		linodeErr, ok := err.(*linodego.Error)
		Expect(ok).To(BeTrue())
		Expect(linodeErr.Code).To(Equal(404))
	}

	type checkArgs struct {
		checkType, path, body, interval, timeout, attempts, checkPassive, protocol, proxyProtocol string
		checkNodes                                                                                bool
	}

	var checkNodeBalancerID = func(service string, expectedID int) {
		err := f.LoadBalancer.WaitForNodeBalancerReady(service, expectedID)
		Expect(err).NotTo(HaveOccurred())
	}

	var checkNodeBalancerConfig = func(args checkArgs) {
		By("Getting NodeBalancer Configuration")
		nbConfig, err := f.LoadBalancer.GetNodeBalancerConfig(framework.TestServerResourceName)
		Expect(err).NotTo(HaveOccurred())

		if args.checkType != "" {
			By("Checking Health Check Type")
			Expect(string(nbConfig.Check) == args.checkType).Should(BeTrue())
		}

		if args.path != "" {
			By("Checking Health Check Path")
			Expect(nbConfig.CheckPath == args.path).Should(BeTrue())
		}

		if args.body != "" {
			By("Checking Health Check Body")
			Expect(nbConfig.CheckBody == args.body).Should(BeTrue())
		}

		if args.interval != "" {
			By("Checking TCP Connection Health Check Body")
			intInterval, err := strconv.Atoi(args.interval)
			Expect(err).NotTo(HaveOccurred())

			Expect(nbConfig.CheckInterval == intInterval).Should(BeTrue())
		}

		if args.timeout != "" {
			By("Checking TCP Connection Health Check Timeout")
			intTimeout, err := strconv.Atoi(args.timeout)
			Expect(err).NotTo(HaveOccurred())

			Expect(nbConfig.CheckTimeout == intTimeout).Should(BeTrue())
		}

		if args.attempts != "" {
			By("Checking TCP Connection Health Check Attempts")
			intAttempts, err := strconv.Atoi(args.attempts)
			Expect(err).NotTo(HaveOccurred())

			Expect(nbConfig.CheckAttempts == intAttempts).Should(BeTrue())
		}

		if args.checkPassive != "" {
			By("Checking for Passive Health Check")
			boolCheckPassive, err := strconv.ParseBool(args.checkPassive)
			Expect(err).NotTo(HaveOccurred())

			Expect(nbConfig.CheckPassive == boolCheckPassive).Should(BeTrue())
		}

		if args.protocol != "" {
			By("Checking for Protocol")
			Expect(string(nbConfig.Protocol) == args.protocol).Should(BeTrue())
		}

		if args.proxyProtocol != "" {
			By("Checking for Proxy Protocol")
			Expect(string(nbConfig.ProxyProtocol) == args.proxyProtocol).Should(BeTrue())
		}

		if args.checkNodes {
			checkNumberOfUpNodes(2)
		}
	}

	var addNewNode = func() {
		_, err := sh.Command("terraform", "apply", "-var", "nodes=3", "-auto-approve").Output()
		Expect(err).NotTo(HaveOccurred())
	}

	var deleteNewNode = func() {
		_, err := sh.Command("terraform", "apply", "-var", "nodes=2", "-auto-approve").Output()
		Expect(err).NotTo(HaveOccurred())
	}

	var waitForNodeAddition = func() {
		checkNumberOfUpNodes(3)
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
					ports := []core.ContainerPort{
						{
							Name:          "http-1",
							ContainerPort: 8080,
						},
					}
					servicePorts := []core.ServicePort{
						{
							Name:       "http-1",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
							Protocol:   "TCP",
						},
					}
					labels = map[string]string{
						"app": "test-loadbalancer",
					}

					By("Creating Pods")
					createPodWithLabel(pods, ports, framework.TestServerImage, labels, true)

					By("Creating Service")
					createServiceWithSelector(labels, servicePorts, false)
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
					err = wait.PollImmediate(framework.RetryInterval, framework.RetryTimeout, func() (bool, error) {
						resp, err := sh.Command("curl", "--max-time", "5", "-s", eps[0]).Output()
						if err != nil {
							return false, nil
						}
						stringResp := string(resp)
						if strings.Contains(stringResp, pods[0]) {
							log.Println("Got response from " + pods[0])
							counter1++
						} else if strings.Contains(stringResp, pods[1]) {
							log.Println("Got response from " + pods[1])
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

	FDescribe("Test", func() {
		Context("LoadBalancer", func() {
			AfterEach(func() {
				err := root.Recycle()
				Expect(err).NotTo(HaveOccurred())
			})

			Context("With single TLS port", func() {
				var (
					pods        []string
					labels      map[string]string
					annotations map[string]string
					secretName  string
				)
				BeforeEach(func() {
					pods = []string{"test-single-port-pod"}
					ports := []core.ContainerPort{
						{
							Name:          "https",
							ContainerPort: 8080,
						},
					}
					servicePorts := []core.ServicePort{
						{
							Name:       "https",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
							Protocol:   "TCP",
						},
					}
					secretName = "tls-secret"
					labels = map[string]string{
						"app": "test-loadbalancer",
					}
					annotations = map[string]string{
						annLinodePortConfigPrefix + "80": `{ "tls-secret-name": "` + secretName + `" }`,
						annLinodeDefaultProtocol:         "https",
					}

					By("Creating Pod")
					createPodWithLabel(pods, ports, framework.TestServerImage, labels, false)

					By("Creating Secret")
					err = f.LoadBalancer.CreateTLSSecret("tls-secret")
					Expect(err).NotTo(HaveOccurred())

					By("Creating Service")
					createServiceWithAnnotations(labels, annotations, servicePorts, false)
				})

				AfterEach(func() {
					By("Deleting the Secrets")
					deletePods(pods)

					By("Deleting the Service")
					deleteService()

					By("Deleting the Secret")
					deleteSecret(secretName)
				})

				It("should reach the pod via tls", func() {
					By("Checking TCP Response")
					eps, err := f.LoadBalancer.GetHTTPEndpoints()
					Expect(err).NotTo(HaveOccurred())
					Expect(len(eps)).Should(BeNumerically(">=", 1))

					By("Waiting for Response from the LoadBalancer url: " + eps[0])
					err = framework.WaitForHTTPSResponse(eps[0], pods[0])
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("With ProxyProtocol", func() {
				var (
					pods         []string
					labels       map[string]string
					servicePorts []core.ServicePort

					annotations = map[string]string{}
				)
				BeforeEach(func() {
					pods = []string{"test-pod-1"}
					ports := []core.ContainerPort{
						{
							Name:          "http-1",
							ContainerPort: 8080,
						},
					}
					servicePorts = []core.ServicePort{
						{
							Name:       "http-1",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
							Protocol:   "TCP",
						},
					}

					labels = map[string]string{
						"app": "test-loadbalancer-with-proxyprotocol",
					}

					By("Creating Pod")
					createPodWithLabel(pods, ports, framework.TestServerImage, labels, false)

					By("Creating Service")
					createServiceWithAnnotations(labels, annotations, servicePorts, false)
				})

				AfterEach(func() {
					By("Deleting the Pods")
					deletePods(pods)

					By("Deleting the Service")
					deleteService()
				})

				It("should update the NodeBalancer to use ProxyProtocol v2", func() {
					proxyProtocolV2 := string(linodego.ProxyProtocolV2)

					By("Annotating ProxyProtocol v2")
					annotations[annLinodeProxyProtocol] = proxyProtocolV2
					updateServiceWithAnnotations(labels, annotations, servicePorts, false)

					By("Checking NodeBalancerConfig")
					checkNodeBalancerConfig(checkArgs{proxyProtocol: proxyProtocolV2})
				})
			})

			Context("With Multiple HTTP and HTTPS Ports", func() {
				var (
					pods        []string
					labels      map[string]string
					annotations map[string]string
					secretName1 string
					secretName2 string
				)
				BeforeEach(func() {
					pods = []string{"tls-multi-port-pod"}
					secretName1 = "tls-secret-1"
					secretName2 = "tls-secret-2"
					labels = map[string]string{
						"app": "test-loadbalancer",
					}
					annotations = map[string]string{
						annLinodeDefaultProtocol:           "https",
						annLinodePortConfigPrefix + "80":   `{"protocol": "http"}`,
						annLinodePortConfigPrefix + "8080": `{"protocol": "http"}`,
						annLinodePortConfigPrefix + "443":  `{"tls-secret-name": "` + secretName1 + `"}`,
						annLinodePortConfigPrefix + "8443": `{"tls-secret-name": "` + secretName2 + `", "protocol": "https"}`,
					}
					ports := []core.ContainerPort{
						{
							Name:          "alpha",
							ContainerPort: 8080,
						},
						{
							Name:          "beta",
							ContainerPort: 8989,
						},
					}
					servicePorts := []core.ServicePort{
						{
							Name:       "http-1",
							Port:       80,
							TargetPort: intstr.FromInt(8989),
							Protocol:   "TCP",
						},
						{
							Name:       "http-2",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
							Protocol:   "TCP",
						},
						{
							Name:       "https-1",
							Port:       443,
							TargetPort: intstr.FromInt(8080),
							Protocol:   "TCP",
						},
						{
							Name:       "https-2",
							Port:       8443,
							TargetPort: intstr.FromInt(8989),
							Protocol:   "TCP",
						},
					}

					By("Creating Pod")
					createPodWithLabel(pods, ports, framework.TestServerImage, labels, false)

					By("Creating Secret")
					err = f.LoadBalancer.CreateTLSSecret(secretName1)
					Expect(err).NotTo(HaveOccurred())
					err = f.LoadBalancer.CreateTLSSecret(secretName2)
					Expect(err).NotTo(HaveOccurred())

					By("Creating Service")
					createServiceWithAnnotations(labels, annotations, servicePorts, false)
				})

				AfterEach(func() {
					By("Deleting the Secrets")
					deletePods(pods)

					By("Deleting the Service")
					deleteService()

					By("Deleting the Secret")
					deleteSecret(secretName1)
					deleteSecret(secretName2)
				})

				It("should reach the pods", func() {
					By("Checking TCP Response")
					eps, err := f.LoadBalancer.GetHTTPEndpoints()
					Expect(err).NotTo(HaveOccurred())
					Expect(len(eps)).Should(BeNumerically("==", 4))

					// in order of the spec
					http80, http8080, https443, https8443 := eps[0], eps[1], eps[2], eps[3]

					waitForResponse := func(endpoint string, fn func(string, string) error) {
						By("Waiting for Response from the LoadBalancer url: " + endpoint)
						err := fn(endpoint, pods[0])
						Expect(err).NotTo(HaveOccurred())
					}
					waitForResponse(http80, framework.WaitForHTTPResponse)
					waitForResponse(http8080, framework.WaitForHTTPResponse)
					waitForResponse(https443, framework.WaitForHTTPSResponse)
					waitForResponse(https8443, framework.WaitForHTTPSResponse)
				})
			})

			Context("With HTTP updating to have HTTPS", func() {
				var (
					pods        []string
					labels      map[string]string
					annotations map[string]string
					secretName  string
				)
				BeforeEach(func() {
					pods = []string{"tls-pod"}
					secretName = "tls-secret-1"
					labels = map[string]string{
						"app": "test-loadbalancer",
					}
					annotations = map[string]string{
						annLinodeDefaultProtocol:         "https",
						annLinodePortConfigPrefix + "80": `{"protocol": "http"}`,
					}
					ports := []core.ContainerPort{
						{
							Name:          "alpha",
							ContainerPort: 8080,
						},
					}
					servicePorts := []core.ServicePort{
						{
							Name:       "http",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
							Protocol:   "TCP",
						},
					}

					By("Creating Pod")
					createPodWithLabel(pods, ports, framework.TestServerImage, labels, false)

					By("Creating Service")
					createServiceWithAnnotations(labels, annotations, servicePorts, false)

					By("Creating Secret")
					err = f.LoadBalancer.CreateTLSSecret(secretName)
					Expect(err).NotTo(HaveOccurred())

					By("Updating the Service")
					updateAnnotations := map[string]string{
						annLinodeDefaultProtocol:          "https",
						annLinodePortConfigPrefix + "80":  `{"protocol": "http"}`,
						annLinodePortConfigPrefix + "443": `{"tls-secret-name": "` + secretName + `", "protocol": "https"}`,
					}
					updateServicePorts := []core.ServicePort{
						{
							Name:       "http",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
							Protocol:   "TCP",
						},
						{
							Name:       "https",
							Port:       443,
							TargetPort: intstr.FromInt(8080),
							Protocol:   "TCP",
						},
					}
					updateServiceWithAnnotations(labels, updateAnnotations, updateServicePorts, false)
				})

				AfterEach(func() {
					By("Deleting the Secrets")
					deletePods(pods)

					By("Deleting the Service")
					deleteService()

					By("Deleting the Secret")
					deleteSecret(secretName)
				})

				It("should reach the pods", func() {
					By("Checking TCP Response")
					eps, err := f.LoadBalancer.GetHTTPEndpoints()
					Expect(err).NotTo(HaveOccurred())
					Expect(len(eps)).Should(BeNumerically("==", 2))

					// in order of the spec
					http80, https443 := eps[0], eps[1]

					waitForResponse := func(endpoint string, fn func(string, string) error) {
						By("Waiting for Response from the LoadBalancer url: " + endpoint)
						err := fn(endpoint, pods[0])
						Expect(err).NotTo(HaveOccurred())
					}
					waitForResponse(http80, framework.WaitForHTTPResponse)
					waitForResponse(https443, framework.WaitForHTTPSResponse)
				})
			})

			Context("With SessionAffinity", func() {
				var (
					pods   []string
					labels map[string]string
				)

				BeforeEach(func() {
					pods = []string{"test-pod-1", "test-pod-2"}
					ports := []core.ContainerPort{
						{
							Name:          "http-1",
							ContainerPort: 8080,
						},
					}
					servicePorts := []core.ServicePort{
						{
							Name:       "http-1",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
							Protocol:   "TCP",
						},
					}
					labels = map[string]string{
						"app": "test-loadbalancer",
					}

					By("Creating Pods")
					createPodWithLabel(pods, ports, framework.TestServerImage, labels, false)

					By("Creating Service")
					createServiceWithSelector(labels, servicePorts, true)
				})

				AfterEach(func() {
					By("Deleting the Pods")
					deletePods(pods)

					By("Deleting the Service")
					deleteService()
				})

				It("should reach the same pod every time it requests", func() {
					By("Checking TCP Response")
					eps, err := f.LoadBalancer.GetHTTPEndpoints()
					Expect(err).NotTo(HaveOccurred())
					Expect(len(eps)).Should(BeNumerically(">=", 1))

					By("Waiting for Response from the LoadBalancer url: " + eps[0])
					getResponseFromSamePod(eps[0])
				})
			})

			Context("For HTTP body health check", func() {
				var (
					pods        []string
					labels      map[string]string
					annotations map[string]string

					checkType = "http_body"
					path      = "/"
					body      = "nginx"
					protocol  = "http"
				)
				BeforeEach(func() {
					pods = []string{"test-pod-http-body"}
					ports := []core.ContainerPort{
						{
							Name:          "http",
							ContainerPort: 80,
						},
					}
					servicePorts := []core.ServicePort{
						{
							Name:       "http",
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   "TCP",
						},
					}

					labels = map[string]string{
						"app": "test-loadbalancer",
					}
					annotations = map[string]string{
						annLinodeHealthCheckType: checkType,
						annLinodeCheckPath:       path,
						annLinodeCheckBody:       body,
						annLinodeDefaultProtocol: protocol,
					}

					By("Creating Pod")
					createPodWithLabel(pods, ports, "nginx", labels, false)

					By("Creating Service")
					createServiceWithAnnotations(labels, annotations, servicePorts, false)
				})

				AfterEach(func() {
					By("Deleting the Pods")
					deletePods(pods)

					By("Deleting the Service")
					deleteService()
				})

				It("should successfully check the health of 2 nodes", func() {
					By("Checking NodeBalancer Configurations")
					checkNodeBalancerConfig(checkArgs{
						checkType:  checkType,
						path:       path,
						body:       body,
						protocol:   protocol,
						checkNodes: true,
					})
				})
			})

			Context("Updated with NodeBalancerID", func() {
				var (
					pods         []string
					labels       map[string]string
					servicePorts []core.ServicePort

					annotations = map[string]string{}
				)
				BeforeEach(func() {
					pods = []string{"test-pod-1"}
					ports := []core.ContainerPort{
						{
							Name:          "http-1",
							ContainerPort: 8080,
						},
					}
					servicePorts = []core.ServicePort{
						{
							Name:       "http-1",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
							Protocol:   "TCP",
						},
					}

					labels = map[string]string{
						"app": "test-loadbalancer-with-nodebalancer-id",
					}

					By("Creating Pod")
					createPodWithLabel(pods, ports, framework.TestServerImage, labels, false)

					By("Creating Service")
					createServiceWithAnnotations(labels, annotations, servicePorts, false)
				})

				AfterEach(func() {
					By("Deleting the Pods")
					deletePods(pods)

					By("Deleting the Service")
					deleteService()
				})

				It("should use the specified NodeBalancer", func() {
					By("Creating new NodeBalancer")
					nbID := createNodeBalancer()

					By("Annotating service with new NodeBalancer ID")
					annotations[annLinodeNodeBalancerID] = strconv.Itoa(nbID)
					updateServiceWithAnnotations(labels, annotations, servicePorts, false)

					By("Checking the NodeBalancer ID")
					checkNodeBalancerID(framework.TestServerResourceName, nbID)
				})
			})

			Context("Created with NodeBalancerID", func() {
				var (
					pods         []string
					labels       map[string]string
					annotations  map[string]string
					servicePorts []core.ServicePort

					nodeBalancerID int
				)

				BeforeEach(func() {
					pods = []string{"test-pod-1"}
					ports := []core.ContainerPort{
						{
							Name:          "http-1",
							ContainerPort: 8080,
						},
					}
					servicePorts = []core.ServicePort{
						{
							Name:       "http-1",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
							Protocol:   "TCP",
						},
					}

					labels = map[string]string{
						"app": "test-loadbalancer-with-nodebalancer-id",
					}

					By("Creating NodeBalancer")
					nodeBalancerID = createNodeBalancer()

					annotations = map[string]string{
						annLinodeNodeBalancerID: strconv.Itoa(nodeBalancerID),
					}

					By("Creating Pod")
					createPodWithLabel(pods, ports, framework.TestServerImage, labels, false)

					By("Creating Service")
					createServiceWithAnnotations(labels, annotations, servicePorts, false)
				})

				AfterEach(func() {
					By("Deleting the Pods")
					deletePods(pods)

					By("Deleting the Service")
					deleteService()

					err := root.Recycle()
					Expect(err).NotTo(HaveOccurred())
				})

				It("should use the specified NodeBalancer", func() {
					By("Checking the NodeBalancerID")
					checkNodeBalancerID(framework.TestServerResourceName, nodeBalancerID)
				})

				It("should use the newly specified NodeBalancer ID", func() {
					By("Creating new NodeBalancer")
					nbID := createNodeBalancer()

					By("Waiting for currenct NodeBalancer to be ready")
					checkNodeBalancerID(framework.TestServerResourceName, nodeBalancerID)

					By("Annotating service with new NodeBalancer ID")
					annotations[annLinodeNodeBalancerID] = strconv.Itoa(nbID)
					updateServiceWithAnnotations(labels, annotations, servicePorts, false)

					By("Checking the NodeBalancer ID")
					checkNodeBalancerID(framework.TestServerResourceName, nbID)

					By("Checking old NodeBalancer was deleted")
					checkNodeBalancerNotExists(nodeBalancerID)
				})
			})

			Context("With Preserve Annotation", func() {
				var (
					pods           []string
					servicePorts   []core.ServicePort
					labels         map[string]string
					annotations    map[string]string
					nodeBalancerID int
				)

				BeforeEach(func() {
					pods = []string{"test-pod-1"}
					ports := []core.ContainerPort{
						{
							Name:          "http-1",
							ContainerPort: 8080,
						},
					}
					servicePorts = []core.ServicePort{
						{
							Name:       "http-1",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
							Protocol:   "TCP",
						},
					}

					labels = map[string]string{
						"app": "test-loadbalancer",
					}
					annotations = map[string]string{
						annLinodeLoadBalancerPreserve: "true",
					}

					By("Creating Pod")
					createPodWithLabel(pods, ports, framework.TestServerImage, labels, false)

					By("Creating Service")
					createServiceWithAnnotations(labels, annotations, servicePorts, false)

					By("Getting NodeBalancer ID")
					nodeBalancerID, err = f.LoadBalancer.GetNodeBalancerID(framework.TestServerResourceName)
					Expect(err).NotTo(HaveOccurred())
				})

				AfterEach(func() {
					By("Deleting the NodeBalancer")
					deleteNodeBalancer(nodeBalancerID)

					err := root.Recycle()
					Expect(err).NotTo(HaveOccurred())
				})

				It("should preserve the underlying nodebalancer after service deletion", func() {
					By("Deleting the Pods")
					deletePods(pods)

					By("Deleting the Service")
					deleteService()

					By("Checking if the NodeBalancer exists")
					checkNodeBalancerExists(nodeBalancerID)
				})

				It("should preserve the underlying nodebalancer after a new one is specified", func() {
					defer func() {
						By("Deleting the Pods")
						deletePods(pods)

						By("Deleting the Service")
						deleteService()
					}()

					By("Creating new NodeBalancer")
					newID := createNodeBalancer()
					defer func() {
						By("Deleting new NodeBalancer")
						deleteNodeBalancer(newID)
					}()

					By("Annotating service with new NodeBalancer ID")
					annotations[annLinodeNodeBalancerID] = strconv.Itoa(newID)
					updateServiceWithAnnotations(labels, annotations, servicePorts, false)

					By("Checking the service's NodeBalancer ID")
					checkNodeBalancerID(framework.TestServerResourceName, newID)

					By("Checking the old NodeBalancer exists")
					checkNodeBalancerExists(nodeBalancerID)
				})

			})

			Context("With Node Addition", func() {
				var (
					pods   []string
					labels map[string]string
				)

				BeforeEach(func() {
					pods = []string{"test-pod-node-add"}
					ports := []core.ContainerPort{
						{
							Name:          "http-1",
							ContainerPort: 8080,
						},
					}
					servicePorts := []core.ServicePort{
						{
							Name:       "http-1",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
							Protocol:   "TCP",
						},
					}
					labels = map[string]string{
						"app": "test-loadbalancer",
					}

					By("Creating Pods")
					createPodWithLabel(pods, ports, framework.TestServerImage, labels, false)

					By("Creating Service")
					createServiceWithSelector(labels, servicePorts, false)
				})

				AfterEach(func() {
					By("Deleting the Pods")
					deletePods(pods)

					By("Deleting the Service")
					deleteService()

					By("Deleting the Newly Created Nodes")
					deleteNewNode()

					By("Waiting for the Node to be removed")
					checkNumberOfWorkerNodes(2)
				})

				It("should reach the same pod every time it requests", func() {
					By("Adding a New Node")
					addNewNode()

					By("Waiting for the Node to be Added to the NodeBalancer")
					waitForNodeAddition()
				})
			})

			Context("For TCP Connection health check", func() {
				var (
					pods        []string
					labels      map[string]string
					annotations map[string]string

					checkType = "connection"
					interval  = "10"
					timeout   = "5"
					attempts  = "4"
					protocol  = "tcp"
				)
				BeforeEach(func() {
					pods = []string{"test-pod-tcp"}
					ports := []core.ContainerPort{
						{
							Name:          "http",
							ContainerPort: 80,
						},
					}
					servicePorts := []core.ServicePort{
						{
							Name:       "http",
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   "TCP",
						},
					}

					labels = map[string]string{
						"app": "test-loadbalancer",
					}
					annotations = map[string]string{
						annLinodeHealthCheckType:     checkType,
						annLinodeDefaultProtocol:     protocol,
						annLinodeHealthCheckInterval: interval,
						annLinodeHealthCheckTimeout:  timeout,
						annLinodeHealthCheckAttempts: attempts,
					}

					By("Creating Pod")
					createPodWithLabel(pods, ports, "nginx", labels, false)

					By("Creating Service")
					createServiceWithAnnotations(labels, annotations, servicePorts, false)
				})

				AfterEach(func() {
					By("Deleting the Pods")
					deletePods(pods)

					By("Deleting the Service")
					deleteService()
				})

				It("should successfully check the health of 2 nodes", func() {
					By("Checking NodeBalancer Configurations")
					checkNodeBalancerConfig(checkArgs{
						checkType:  checkType,
						interval:   interval,
						timeout:    timeout,
						attempts:   attempts,
						protocol:   protocol,
						checkNodes: true,
					})
				})
			})

			Context("For Passive Health Check", func() {
				var (
					pods        []string
					labels      map[string]string
					annotations map[string]string

					checkType    = "none"
					checkPassive = "true"
				)
				BeforeEach(func() {
					pods = []string{"test-pod-passive-hc"}
					ports := []core.ContainerPort{
						{
							Name:          "http",
							ContainerPort: 80,
						},
					}
					servicePorts := []core.ServicePort{
						{
							Name:       "http",
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   "TCP",
						},
					}

					labels = map[string]string{
						"app": "test-loadbalancer",
					}
					annotations = map[string]string{
						annLinodeHealthCheckPassive: checkPassive,
						annLinodeHealthCheckType:    checkType,
					}

					By("Creating Pod")
					createPodWithLabel(pods, ports, "nginx", labels, false)

					By("Creating Service")
					createServiceWithAnnotations(labels, annotations, servicePorts, false)
				})

				AfterEach(func() {
					By("Deleting the Pods")
					deletePods(pods)

					By("Deleting the Service")
					deleteService()
				})

				It("should successfully check the health of 2 nodes", func() {
					By("Checking NodeBalancer Configurations")
					checkNodeBalancerConfig(checkArgs{
						checkType:    checkType,
						checkPassive: checkPassive,
						checkNodes:   true,
					})
				})
			})

			Context("For HTTP Status Health Check", func() {
				var (
					pods        []string
					labels      map[string]string
					annotations map[string]string

					checkType = "http"
					path      = "/"
				)
				BeforeEach(func() {
					pods = []string{"test-pod-http-status"}
					ports := []core.ContainerPort{
						{
							Name:          "http",
							ContainerPort: 80,
						},
					}
					servicePorts := []core.ServicePort{
						{
							Name:       "http",
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   "TCP",
						},
					}

					labels = map[string]string{
						"app": "test-loadbalancer",
					}
					annotations = map[string]string{
						annLinodeHealthCheckType: checkType,
						annLinodeCheckPath:       path,
						annLinodeDefaultProtocol: "http",
					}

					By("Creating Pod")
					createPodWithLabel(pods, ports, "nginx", labels, false)

					By("Creating Service")
					createServiceWithAnnotations(labels, annotations, servicePorts, false)
				})

				AfterEach(func() {
					By("Deleting the Pods")
					deletePods(pods)

					By("Deleting the Service")
					deleteService()
				})

				It("should successfully check the health of 2 nodes", func() {
					By("Checking NodeBalancer Configurations")
					checkNodeBalancerConfig(checkArgs{
						checkType:  checkType,
						path:       path,
						checkNodes: true,
					})
				})
			})
		})
	})
})
