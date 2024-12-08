package test

import (
	"e2e_test/test/framework"
	"flag"
	"os"
	"testing"
	"time"

	"github.com/linode/linodego"

	"github.com/appscode/go/crypto/rand"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	clusterName string
	region      = "us-east"
	k8s_version string
	linodeURL   = "https://api.linode.com"
)

func init() {
	flag.StringVar(&framework.Image, "image", framework.Image, "registry/repository:tag")
	flag.StringVar(&framework.ApiToken, "api-token", os.Getenv("LINODE_API_TOKEN"), "linode api token")
	flag.StringVar(&framework.KubeConfigFile, "kubeconfig", os.Getenv("TEST_KUBECONFIG"), "To use existing cluster provide kubeconfig file")
	flag.StringVar(&region, "region", region, "Region to create load balancers")
	flag.StringVar(&k8s_version, "k8s_version", k8s_version, "k8s_version for child cluster")
	flag.DurationVar(&framework.Timeout, "timeout", 5*time.Minute, "Timeout for a test to complete successfully")
	flag.StringVar(&linodeURL, "linode-url", linodeURL, "The Linode API URL to send requests to")
}

const (
	TIMEOUT = 5 * time.Minute
)

var root *framework.Framework

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(framework.Timeout)
	RunSpecs(t, "e2e Suite")
}

var getLinodeClient = func() *linodego.Client {
	linodeClient := linodego.NewClient(nil)
	linodeClient.SetToken(framework.ApiToken)
	linodeClient.SetBaseURL(linodeURL)
	return &linodeClient
}

var _ = BeforeSuite(func() {
	clusterName = rand.WithUniqSuffix("ccm-linode")

	By("Using kubeconfig from " + framework.KubeConfigFile)
	config, err := clientcmd.BuildConfigFromFlags("", framework.KubeConfigFile)
	Expect(err).NotTo(HaveOccurred())

	// Clients
	kubeClient := kubernetes.NewForConfigOrDie(config)
	linodeClient := getLinodeClient()

	// Framework
	root = framework.New(config, kubeClient, *linodeClient)

	By("Using Namespace " + root.Namespace())
	err = root.CreateNamespace()
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	By("Deleting Namespace " + root.Namespace())
	err := root.DeleteNamespace()
	Expect(err).NotTo(HaveOccurred())

	By("Not deleting cluster")
})
