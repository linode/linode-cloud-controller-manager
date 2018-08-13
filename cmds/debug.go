package cmds

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/appscode/go/hold"
	"github.com/spf13/cobra"
	_ "k8s.io/kubernetes/pkg/client/metrics/prometheus" // for client metric registration
	_ "k8s.io/kubernetes/pkg/version/prometheus"        // for version metric registration
)

func NewCmdDebug() *cobra.Command {
	var cloudConfigFile string
	cmd := &cobra.Command{
		Use:               "debug",
		Short:             "Debug cloud-controller-manager",
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			contents, err := ioutil.ReadFile(cloudConfigFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to read cloud config %s: Reason: %v", cloudConfigFile, err)
			}
			fmt.Println(contents)
			hold.Hold()
		},
	}
	cmd.Flags().StringVar(&cloudConfigFile, "cloud-config", cloudConfigFile, "The path to the cloud provider configuration file.  Empty string for no configuration file.")
	return cmd
}
