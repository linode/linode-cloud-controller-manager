package test

import (
	"e2e_test/test/framework"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/linode/linodego"
	"golang.org/x/oauth2"

	"github.com/appscode/go/crypto/rand"
	"github.com/onsi/ginkgo/reporters"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	useExisting = false
	reuse       = false
	clusterName string
)

func init() {
	flag.StringVar(&framework.Image, "image", framework.Image, "registry/repository:tag")
	flag.StringVar(&framework.ApiToken, "api-token", os.Getenv("LINODE_API_TOKEN"), "linode api token")
	flag.BoolVar(&reuse, "reuse", reuse, "Create a cluster and continue to use it")
	flag.BoolVar(&useExisting, "use-existing", useExisting, "Use an existing kubernetes cluster")
	flag.StringVar(&framework.KubeConfigFile, "kubeconfig", filepath.Join(homedir.HomeDir(), ".kube/config"), "To use existing cluster provide kubeconfig file")
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

	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "e2e Suite", []Reporter{junitReporter})

}

var getLinodeClient = func() *linodego.Client {
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: framework.ApiToken})

	oauth2Client := &http.Client{
		Transport: &oauth2.Transport{
			Source: tokenSource,
		},
	}

	linodeClient := linodego.NewClient(oauth2Client)

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
		err := framework.CreateCluster(clusterName)
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
