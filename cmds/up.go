package cmds

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/appscode/go/log"
	_ "github.com/linode/linode-cloud-controller-manager/cloud/linode"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/server/healthz"
	"k8s.io/kubernetes/cmd/cloud-controller-manager/app"
	"k8s.io/kubernetes/cmd/cloud-controller-manager/app/options"
	_ "k8s.io/kubernetes/pkg/client/metrics/prometheus" // for client metric registration
	_ "k8s.io/kubernetes/pkg/version/prometheus"        // for version metric registration
)

func init() {
	healthz.DefaultHealthz()
}

func NewCmdUp() *cobra.Command {
	s, _ := options.NewCloudControllerManagerOptions()
	cmd := &cobra.Command{
		Use:               "up",
		Short:             "Bootstrap as a Kubernetes master or node",
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			err := wait.Poll(3*time.Second, 5*time.Minute, func() (bool, error) {
				addr, err := net.LookupIP("google.com")
				return len(addr) > 0, err
			})
			if err != nil {
				log.Fatalln("Failed to resolve DNS. Reason: %v", err)
			}

			c, err := s.Config()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}

			if err := app.Run(c.Complete()); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
		},
	}
	s.AddFlags(cmd.Flags())
	return cmd
}
