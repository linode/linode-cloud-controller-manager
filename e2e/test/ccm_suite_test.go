package test

import (
	"e2e_test/test/framework"
	"flag"
	"os"
	"path/filepath"
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
	useExisting = false
	reuse       = false
	clusterName string
	region      = "us-east"
	k8s_version string
)

func init() {
	flag.StringVar(&framework.Image, "image", framework.Image, "registry/repository:tag")
	flag.StringVar(&framework.ApiToken, "api-token", os.Getenv("LINODE_API_TOKEN"), "linode api token")
	flag.BoolVar(&reuse, "reuse", reuse, "Create a cluster and continue to use it")
	flag.BoolVar(&useExisting, "use-existing", useExisting, "Use an existing kubernetes cluster")
	flag.StringVar(&framework.KubeConfigFile, "kubeconfig", os.Getenv("TEST_KUBECONFIG"), "To use existing cluster provide kubeconfig file")
	flag.StringVar(&region, "region", region, "Region to create load balancers")
	flag.StringVar(&k8s_version, "k8s_version", k8s_version, "k8s_version for child cluster")

}

const (
	TIMEOUT = 5 * time.Minute
)

var (
	root *framework.Framework
)

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(TIMEOUT)
	RunSpecs(t, "e2e Suite")

}

var getLinodeClient = func() *linodego.Client {
	linodeClient := linodego.NewClient(nil)
	linodeClient.SetToken(framework.ApiToken)
	return &linodeClient
}

var _ = BeforeSuite(func() {
	if reuse {
		clusterName = "ccm-linode-for-reuse"
	} else {
		clusterName = rand.WithUniqSuffix("ccm-linode")
	}

	dir, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())
	kubeConfigFile := filepath.Join(dir, clusterName+".conf")

	if reuse {
		if _, err := os.Stat(kubeConfigFile); !os.IsNotExist(err) {
			useExisting = true
			framework.KubeConfigFile = kubeConfigFile
		}
	}

	if !useExisting {
		err := framework.CreateCluster(clusterName, region, k8s_version)
		Expect(err).NotTo(HaveOccurred())
		framework.KubeConfigFile = kubeConfigFile
	}

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
	if !(useExisting || reuse) {
		By("Deleting cluster")
		err := framework.DeleteCluster(clusterName)
		Expect(err).NotTo(HaveOccurred())
	} else {
		By("Deleting Namespace " + root.Namespace())
		err := root.DeleteNamespace()
		Expect(err).NotTo(HaveOccurred())

		By("Not deleting cluster")
	}
})
