package test

import (
	"context"
	"e2e_test/test/framework"
	"fmt"
	"os/exec"
	"strconv"

	"e2e_test/test/framework"

	"github.com/linode/linodego"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/watch"
)

func EnsuredService() types.GomegaMatcher {
	return And(
		WithTransform(func(e watch.Event) (string, error) {
			event, ok := e.Object.(*core.Event)
			if !ok {
				return "", fmt.Errorf("failed to poll event")
			}
			return event.Reason, nil
		}, Equal("EnsuredLoadBalancer")),
	)
}

var _ = Describe("e2e tests", func() {
	var (
		err     error
		f       *framework.Invocation
		workers []string
	)

	const (
		annLinodeProxyProtocolDeprecated = "service.beta.kubernetes.io/linode-loadbalancer-proxy-protocol"
		annLinodeDefaultProxyProtocol    = "service.beta.kubernetes.io/linode-loadbalancer-default-proxy-protocol"
		annLinodeDefaultProtocol         = "service.beta.kubernetes.io/linode-loadbalancer-default-protocol"
		annLinodePortConfigPrefix        = "service.beta.kubernetes.io/linode-loadbalancer-port-"
		annLinodeLoadBalancerPreserve    = "service.beta.kubernetes.io/linode-loadbalancer-preserve"
		annLinodeHealthCheckType         = "service.beta.kubernetes.io/linode-loadbalancer-check-type"
		annLinodeCheckBody               = "service.beta.kubernetes.io/linode-loadbalancer-check-body"
		annLinodeCheckPath               = "service.beta.kubernetes.io/linode-loadbalancer-check-path"
		annLinodeHealthCheckInterval     = "service.beta.kubernetes.io/linode-loadbalancer-check-interval"
		annLinodeHealthCheckTimeout      = "service.beta.kubernetes.io/linode-loadbalancer-check-timeout"
		annLinodeHealthCheckAttempts     = "service.beta.kubernetes.io/linode-loadbalancer-check-attempts"
		annLinodeHealthCheckPassive      = "service.beta.kubernetes.io/linode-loadbalancer-check-passive"
		annLinodeNodeBalancerID          = "service.beta.kubernetes.io/linode-loadbalancer-nodebalancer-id"
		annLinodeHostnameOnlyIngress     = "service.beta.kubernetes.io/linode-loadbalancer-hostname-only-ingress"
	)

	BeforeEach(func() {
		f = root.Invoke()
		workers, err = f.GetNodeList()
		Expect(err).NotTo(HaveOccurred())
		Expect(len(workers)).Should(BeNumerically(">=", 2))
	})

	createPodWithLabel := func(pods []string, ports []core.ContainerPort, image string, labels map[string]string, selectNode bool) {
		for i, pod := range pods {
			p := f.LoadBalancer.GetPodObject(pod, image, ports, labels)
			if selectNode {
				p = f.LoadBalancer.SetNodeSelector(p, workers[i])
			}
			Expect(f.LoadBalancer.CreatePod(p)).ToNot(BeNil())
			Eventually(f.LoadBalancer.GetPod).WithArguments(p.ObjectMeta.Name, f.LoadBalancer.Namespace()).Should(HaveField("Status.Phase", Equal(core.PodRunning)))
		}
	}

	deletePods := func(pods []string) {
		for _, pod := range pods {
			Expect(f.LoadBalancer.DeletePod(pod)).NotTo(HaveOccurred())
		}
	}

	deleteService := func() {
		Expect(f.LoadBalancer.DeleteService()).NotTo(HaveOccurred())
	}

	deleteSecret := func(name string) {
		Expect(f.LoadBalancer.DeleteSecret(name)).NotTo(HaveOccurred())
	}

	ensureServiceLoadBalancer := func() {
		watcher, err := f.LoadBalancer.GetServiceWatcher()
		Expect(err).NotTo(HaveOccurred())
		Eventually(watcher.ResultChan()).Should(Receive(EnsuredService()))
	}

	createServiceWithSelector := func(selector map[string]string, ports []core.ServicePort, isSessionAffinityClientIP bool) {
		Expect(f.LoadBalancer.CreateService(selector, nil, ports, isSessionAffinityClientIP)).NotTo(HaveOccurred())
		Eventually(f.LoadBalancer.GetServiceEndpoints).Should(Not(BeEmpty()))
		ensureServiceLoadBalancer()
	}

	createServiceWithAnnotations := func(labels, annotations map[string]string, ports []core.ServicePort, isSessionAffinityClientIP bool) {
		Expect(f.LoadBalancer.CreateService(labels, annotations, ports, isSessionAffinityClientIP)).NotTo(HaveOccurred())
		Eventually(f.LoadBalancer.GetServiceEndpoints).Should(Not(BeEmpty()))
		ensureServiceLoadBalancer()
	}

	updateServiceWithAnnotations := func(labels, annotations map[string]string, ports []core.ServicePort, isSessionAffinityClientIP bool) {
		Expect(f.LoadBalancer.UpdateService(labels, annotations, ports, isSessionAffinityClientIP)).NotTo(HaveOccurred())
		Eventually(f.LoadBalancer.GetServiceEndpoints).Should(Not(BeEmpty()))
		ensureServiceLoadBalancer()
	}

	deleteNodeBalancer := func(id int) {
		Expect(getLinodeClient().DeleteNodeBalancer(context.Background(), id)).NotTo(HaveOccurred())
	}

	createNodeBalancer := func() int {
		var nb *linodego.NodeBalancer
		nb, err = getLinodeClient().CreateNodeBalancer(context.TODO(), linodego.NodeBalancerCreateOptions{
			Region: region,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(nb).NotTo(BeNil())
		return nb.ID
	}

	checkNumberOfWorkerNodes := func(numNodes int) {
		Eventually(f.GetNodeList).Should(HaveLen(numNodes))
	}

	checkNumberOfUpNodes := func(numNodes int) {
		By("Checking the Number of Up Nodes")
		Eventually(f.LoadBalancer.GetNodeBalancerUpNodes).WithArguments(framework.TestServerResourceName).Should(BeNumerically(">=", numNodes))
	}

	checkNodeBalancerExists := func(id int) {
		By("Checking if the NodeBalancer exists")
		Eventually(getLinodeClient().GetNodeBalancer).WithArguments(context.Background(), id).Should(HaveField("ID", Equal(id)))
	}

	checkNodeBalancerNotExists := func(id int) {
		Eventually(func() int {
			_, err := getLinodeClient().GetNodeBalancer(context.Background(), id)
			if err == nil {
				return 0
			}
			linodeErr, _ := err.(*linodego.Error)
			return linodeErr.Code
		}).Should(Equal(404))
	}

	type checkArgs struct {
		checkType, path, body, interval, timeout, attempts, checkPassive, protocol, proxyProtocol string
		checkNodes                                                                                bool
	}

	checkNodeBalancerID := func(service string, expectedID int) {
		Eventually(f.LoadBalancer.GetNodeBalancerID).WithArguments(service).Should(Equal(expectedID))
	}

	checkLBStatus := func(service string, hasIP bool) {
		Eventually(f.LoadBalancer.GetNodeBalancerFromService).WithArguments(service, hasIP).Should(Not(BeNil()))
	}

	checkNodeBalancerConfigForPort := func(port int, args checkArgs) {
		By("Getting NodeBalancer Configuration for port " + strconv.Itoa(port))
		var nbConfig *linodego.NodeBalancerConfig
		Eventually(func() error {
			nbConfig, err = f.LoadBalancer.GetNodeBalancerConfigForPort(framework.TestServerResourceName, port)
			return err
		}).Should(BeNil())

		if args.checkType != "" {
			By("Checking Health Check Type")
			Expect(string(nbConfig.Check)).To(Equal(args.checkType))
		}

		if args.path != "" {
			By("Checking Health Check Path")
			Expect(nbConfig.CheckPath).To(Equal(args.path))
		}

		if args.body != "" {
			By("Checking Health Check Body")
			Expect(nbConfig.CheckBody).To(Equal(args.body))
		}

		if args.interval != "" {
			By("Checking TCP Connection Health Check Body")
			intInterval, err := strconv.Atoi(args.interval)
			Expect(err).NotTo(HaveOccurred())

			Expect(nbConfig.CheckInterval).To(Equal(intInterval))
		}

		if args.timeout != "" {
			By("Checking TCP Connection Health Check Timeout")
			intTimeout, err := strconv.Atoi(args.timeout)
			Expect(err).NotTo(HaveOccurred())

			Expect(nbConfig.CheckTimeout).To(Equal(intTimeout))
		}

		if args.attempts != "" {
			By("Checking TCP Connection Health Check Attempts")
			intAttempts, err := strconv.Atoi(args.attempts)
			Expect(err).NotTo(HaveOccurred())

			Expect(nbConfig.CheckAttempts).To(Equal(intAttempts))
		}

		if args.checkPassive != "" {
			By("Checking for Passive Health Check")
			boolCheckPassive, err := strconv.ParseBool(args.checkPassive)
			Expect(err).NotTo(HaveOccurred())

			Expect(nbConfig.CheckPassive).To(Equal(boolCheckPassive))
		}

		if args.protocol != "" {
			By("Checking for Protocol")
			Expect(string(nbConfig.Protocol)).To(Equal(args.protocol))
		}

		if args.proxyProtocol != "" {
			By("Checking for Proxy Protocol")
			Expect(string(nbConfig.ProxyProtocol)).To(Equal(args.proxyProtocol))
		}

		if args.checkNodes {
			checkNumberOfUpNodes(2)
		}
	}

	addNewNode := func() {
		err := exec.Command("terraform", "apply", "-var", "nodes=3", "-auto-approve").Run()
		Expect(err).NotTo(HaveOccurred())
	}

	deleteNewNode := func() {
		err := exec.Command("terraform", "apply", "-var", "nodes=2", "-auto-approve").Run()
		Expect(err).NotTo(HaveOccurred())
	}

	waitForNodeAddition := func() {
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
					var eps []string
					Eventually(func() error {
						eps, err = f.LoadBalancer.GetLoadBalancerIps()
						return err
					}).Should(BeNil())
					Eventually(framework.GetResponseFromCurl).WithArguments(eps[0]).Should(ContainSubstring(pods[0]))
					Eventually(framework.GetResponseFromCurl).WithArguments(eps[0]).Should(ContainSubstring(pods[1]))
				})
			})
		})
	})

	Describe("Test", func() {
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
					Expect(f.LoadBalancer.CreateTLSSecret("tls-secret")).NotTo(HaveOccurred())

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
					var eps []string
					Eventually(func() error {
						eps, err = f.LoadBalancer.GetLoadBalancerIps()
						return err
					}).Should(BeNil())

					By("Waiting for Response from the LoadBalancer url: " + eps[0])
					Eventually(framework.WaitForHTTPSResponse).WithArguments(eps[0]).Should(ContainSubstring(pods[0]))
				})
			})

			Context("With Hostname only ingress", func() {
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
							ContainerPort: 80,
						},
					}
					servicePorts = []core.ServicePort{
						{
							Name:       "http-1",
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   "TCP",
						},
					}

					labels = map[string]string{
						"app": "test-loadbalancer-with-hostname-only-ingress",
					}

					By("Creating Pod")
					createPodWithLabel(pods, ports, framework.TestServerImage, labels, false)

					By("Creating Service")
					createServiceWithAnnotations(labels, map[string]string{}, servicePorts, false)
				})

				AfterEach(func() {
					By("Deleting the Pods")
					deletePods(pods)

					By("Deleting the Service")
					deleteService()
				})

				It("can update service to only use Hostname in ingress", func() {
					By("Checking LB Status has IP")
					checkLBStatus(framework.TestServerResourceName, true)

					By("Annotating service with " + annLinodeHostnameOnlyIngress)
					updateServiceWithAnnotations(labels, map[string]string{
						annLinodeHostnameOnlyIngress: "true",
					}, servicePorts, false)

					By("Checking LB Status does not have IP")
					checkLBStatus(framework.TestServerResourceName, false)
				})

				annotations[annLinodeHostnameOnlyIngress] = "true"

				It("can create a service that only uses Hostname in ingress", func() {
					By("Creating a service annotated with " + annLinodeHostnameOnlyIngress)
					checkLBStatus(framework.TestServerResourceName, true)
				})
			})

			Context("With ProxyProtocol", func() {
				var (
					pods         []string
					labels       map[string]string
					servicePorts []core.ServicePort

					proxyProtocolV1   = string(linodego.ProxyProtocolV1)
					proxyProtocolV2   = string(linodego.ProxyProtocolV2)
					proxyProtocolNone = string(linodego.ProxyProtocolNone)
				)
				BeforeEach(func() {
					pods = []string{"test-pod-1"}
					ports := []core.ContainerPort{
						{
							Name:          "http-1",
							ContainerPort: 80,
						},
						{
							Name:          "http-2",
							ContainerPort: 8080,
						},
					}
					servicePorts = []core.ServicePort{
						{
							Name:       "http-1",
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   "TCP",
						},
						{
							Name:       "http-2",
							Port:       8080,
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
					createServiceWithAnnotations(labels, map[string]string{}, servicePorts, false)
				})

				AfterEach(func() {
					By("Deleting the Pods")
					deletePods(pods)

					By("Deleting the Service")
					deleteService()
				})

				It("can set proxy-protocol on each port", func() {
					By("Annotating port 80 with v1 and 8080 with v2")
					updateServiceWithAnnotations(labels, map[string]string{
						annLinodePortConfigPrefix + "80":   fmt.Sprintf(`{"proxy-protocol": "%s"}`, proxyProtocolV1),
						annLinodePortConfigPrefix + "8080": fmt.Sprintf(`{"proxy-protocol": "%s"}`, proxyProtocolV2),
					}, servicePorts, false)

					By("Checking NodeBalancerConfig for port 80 should have ProxyProtocol v1")
					checkNodeBalancerConfigForPort(80, checkArgs{proxyProtocol: proxyProtocolV1})

					By("Checking NodeBalancerConfig for port 8080 should have ProxyProtocol v2")
					checkNodeBalancerConfigForPort(8080, checkArgs{proxyProtocol: proxyProtocolV2})
				})

				It("should override default proxy-protocol annotation when a port configuration is specified", func() {
					By("Annotating a default version of ProxyProtocol v2 and v1 for port 8080")
					updateServiceWithAnnotations(labels, map[string]string{
						annLinodeDefaultProxyProtocol:      proxyProtocolV2,
						annLinodePortConfigPrefix + "8080": fmt.Sprintf(`{"proxy-protocol": "%s"}`, proxyProtocolV1),
					}, servicePorts, false)

					By("Checking NodeBalancerConfig for port 80 should have the default ProxyProtocol v2")
					checkNodeBalancerConfigForPort(80, checkArgs{proxyProtocol: proxyProtocolV2})

					By("Checking NodeBalancerConfig for port 8080 should have ProxyProtocol v1")
					checkNodeBalancerConfigForPort(8080, checkArgs{proxyProtocol: proxyProtocolV1})
				})

				It("port specific configuration should not effect other ports", func() {
					By("Annotating ProxyProtocol v2 on port 8080")
					updateServiceWithAnnotations(labels, map[string]string{
						annLinodePortConfigPrefix + "8080": fmt.Sprintf(`{"proxy-protocol": "%s"}`, proxyProtocolV2),
					}, servicePorts, false)

					By("Checking NodeBalancerConfig for port 8080 should have ProxyProtocolv2")
					checkNodeBalancerConfigForPort(8080, checkArgs{proxyProtocol: proxyProtocolV2})

					By("Checking NodeBalancerConfig for port 80 should not have ProxyProtocol enabled")
					checkNodeBalancerConfigForPort(80, checkArgs{proxyProtocol: proxyProtocolNone})
				})

				It("default annotations can be used to apply ProxyProtocol to all NodeBalancerConfigs", func() {
					annotations := make(map[string]string)

					By("By specifying ProxyProtocol v2 using the deprecated annotation " + annLinodeProxyProtocolDeprecated)
					annotations[annLinodeProxyProtocolDeprecated] = proxyProtocolV2
					updateServiceWithAnnotations(labels, annotations, servicePorts, false)

					By("Checking NodeBalancerConfig for port 80 should have default ProxyProtocol v2")
					checkNodeBalancerConfigForPort(80, checkArgs{proxyProtocol: proxyProtocolV2})
					By("Checking NodeBalancerConfig for port 8080 should have ProxyProtocol v2")
					checkNodeBalancerConfigForPort(8080, checkArgs{proxyProtocol: proxyProtocolV2})

					By("specifying ProxyProtocol v1 using the annotation " + annLinodeDefaultProtocol)
					annotations[annLinodeDefaultProxyProtocol] = proxyProtocolV1
					updateServiceWithAnnotations(labels, annotations, servicePorts, false)

					By("Checking NodeBalancerConfig for port 80 should have default ProxyProtocol v1")
					checkNodeBalancerConfigForPort(80, checkArgs{proxyProtocol: proxyProtocolV1})
					By("Checking NodeBalancerConfig for port 8080 should have ProxyProtocol v1")
					checkNodeBalancerConfigForPort(8080, checkArgs{proxyProtocol: proxyProtocolV1})
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
					var eps []string
					Eventually(func() error {
						eps, err = f.LoadBalancer.GetLoadBalancerIps()
						return err
					}).Should(BeNil())
					Expect(eps).Should(HaveLen(4))

					// in order of the spec
					http80, http8080, https443, https8443 := eps[0], eps[1], eps[2], eps[3]
					Eventually(framework.WaitForHTTPResponse).WithArguments(http80).Should(ContainSubstring(pods[0]))
					Eventually(framework.WaitForHTTPResponse).WithArguments(http8080).Should(ContainSubstring(pods[0]))
					Eventually(framework.WaitForHTTPSResponse).WithArguments(https443).Should(ContainSubstring(pods[0]))
					Eventually(framework.WaitForHTTPSResponse).WithArguments(https8443).Should(ContainSubstring(pods[0]))
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
					var eps []string
					Eventually(func() error {
						eps, err = f.LoadBalancer.GetLoadBalancerIps()
						return err
					}).Should(BeNil())
					Expect(eps).Should(HaveLen(2))
					http80, https443 := eps[0], eps[1]
					By("Waiting for Response from the LoadBalancer url: " + http80)
					Eventually(framework.WaitForHTTPResponse).WithArguments(http80).Should(ContainSubstring(pods[0]))

					By("Waiting for Response from the LoadBalancer url: " + https443)
					Eventually(framework.WaitForHTTPSResponse).WithArguments(https443).Should(ContainSubstring(pods[0]))
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
					checkNodeBalancerConfigForPort(80, checkArgs{
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
					checkNodeBalancerConfigForPort(80, checkArgs{
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
					checkNodeBalancerConfigForPort(80, checkArgs{
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
					checkNodeBalancerConfigForPort(80, checkArgs{
						checkType:  checkType,
						path:       path,
						checkNodes: true,
					})
				})
			})
		})
	})
})
