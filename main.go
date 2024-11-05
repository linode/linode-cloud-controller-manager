package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"

	"k8s.io/component-base/logs"

	"github.com/linode/linode-cloud-controller-manager/cloud/linode"
	"github.com/linode/linode-cloud-controller-manager/sentry"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/wait"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/cloud-provider/app"
	"k8s.io/cloud-provider/app/config"
	"k8s.io/cloud-provider/names"
	"k8s.io/cloud-provider/options"
	utilflag "k8s.io/component-base/cli/flag"
	"k8s.io/klog/v2"

	_ "k8s.io/component-base/metrics/prometheus/clientgo" // for client metric registration
	_ "k8s.io/component-base/metrics/prometheus/version"  // for version metric registration
)

const (
	sentryDSNVariable            = "SENTRY_DSN"
	sentryEnvironmentVariable    = "SENTRY_ENVIRONMENT"
	sentryReleaseVariable        = "SENTRY_RELEASE"
	linodeExternalSubnetVariable = "LINODE_EXTERNAL_SUBNET"
)

func initializeSentry() {
	var (
		dsn         string
		environment string
		release     string
		ok          bool
	)

	if dsn, ok = os.LookupEnv(sentryDSNVariable); !ok {
		fmt.Printf("%s not set, not initializing Sentry\n", sentryDSNVariable)
		return
	}

	if environment, ok = os.LookupEnv(sentryEnvironmentVariable); !ok {
		fmt.Printf("%s not set, not initializing Sentry\n", sentryEnvironmentVariable)
		return
	}

	if release, ok = os.LookupEnv(sentryReleaseVariable); !ok {
		fmt.Printf("%s not set, defaulting to unknown", sentryReleaseVariable)
		release = "unknown"
	}

	if err := sentry.Initialize(dsn, environment, release); err != nil {
		fmt.Printf("error initializing sentry: %s\n", err.Error())
		return
	}

	fmt.Print("Sentry successfully initialized\n")
}

func main() {
	fmt.Printf("Linode Cloud Controller Manager starting up\n")

	initializeSentry()

	ctx := sentry.SetHubOnContext(context.Background())

	ccmOptions, err := options.NewCloudControllerManagerOptions()
	if err != nil {
		klog.Fatalf("unable to initialize command options: %v", err)
	}
	fss := utilflag.NamedFlagSets{}
	controllerAliases := names.CCMControllerAliases()
	command := app.NewCloudControllerManagerCommand(ccmOptions, cloudInitializer, app.DefaultInitFuncConstructors, controllerAliases, fss, wait.NeverStop)

	// Add Linode-specific flags
	command.Flags().BoolVar(&linode.Options.LinodeGoDebug, "linodego-debug", false, "enables debug output for the LinodeAPI wrapper")
	command.Flags().BoolVar(&linode.Options.EnableRouteController, "enable-route-controller", false, "enables route_controller for ccm")
	command.Flags().StringVar(&linode.Options.VPCName, "vpc-name", "", "[deprecated] vpc name whose routes will be managed by route-controller")
	command.Flags().StringVar(&linode.Options.VPCNames, "vpc-names", "", "comma separated vpc names whose routes will be managed by route-controller")
	command.Flags().StringVar(&linode.Options.LoadBalancerType, "load-balancer-type", "nodebalancer", "configures which type of load-balancing to use for LoadBalancer Services (options: nodebalancer, cilium-bgp)")
	command.Flags().StringVar(&linode.Options.BGPNodeSelector, "bgp-node-selector", "", "node selector to use to perform shared IP fail-over with BGP (e.g. cilium-bgp-peering=true")

	// Set static flags
	command.Flags().VisitAll(func(fl *pflag.Flag) {
		var err error
		switch fl.Name {
		case "cloud-provider":
			err = fl.Value.Set(linode.ProviderName)
		case
			// Prevent reaching out to an authentication-related ConfigMap that
			// we do not need, and thus do not intend to create RBAC permissions
			// for. See also
			// https://github.com/linode/linode-cloud-controller-manager/issues/91
			// and https://github.com/kubernetes/cloud-provider/issues/29.
			"authentication-skip-lookup":
			err = fl.Value.Set("true")
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to set flag %q: %s\n", fl.Name, err)
			os.Exit(1)
		}
	})

	// Make the Linode-specific CCM bits aware of the kubeconfig flag
	linode.Options.KubeconfigFlag = command.Flags().Lookup("kubeconfig")
	if linode.Options.KubeconfigFlag == nil {
		msg := "kubeconfig missing from CCM flag set"
		sentry.CaptureError(ctx, fmt.Errorf(msg))
		fmt.Fprintf(os.Stderr, "kubeconfig missing from CCM flag set"+"\n")
		os.Exit(1)
	}

	if externalSubnet, ok := os.LookupEnv(linodeExternalSubnetVariable); ok && externalSubnet != "" {
		_, network, err := net.ParseCIDR(externalSubnet)
		if err != nil {
			msg := fmt.Sprintf("Unable to parse %s as network subnet: %v", externalSubnet, err)
			sentry.CaptureError(ctx, fmt.Errorf(msg))
			fmt.Fprintf(os.Stderr, "%v\n", msg)
			os.Exit(1)
		}
		linode.Options.LinodeExternalNetwork = network
	}

	pflag.CommandLine.SetNormalizeFunc(utilflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	logs.InitLogs()
	defer logs.FlushLogs()

	if err := command.Execute(); err != nil {
		sentry.CaptureError(ctx, err)
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func cloudInitializer(config *config.CompletedConfig) cloudprovider.Interface {
	// initialize cloud provider with the cloud provider name and config file provided
	cloud, err := cloudprovider.InitCloudProvider(linode.ProviderName, "")
	if err != nil {
		klog.Fatalf("Cloud provider could not be initialized: %v", err)
	}
	if cloud == nil {
		klog.Fatalf("Cloud provider is nil")
	}

	if !cloud.HasClusterID() {
		if config.ComponentConfig.KubeCloudShared.AllowUntaggedCloud {
			klog.Warning("detected a cluster without a ClusterID.  A ClusterID will be required in the future.  Please tag your cluster to avoid any future issues")
		} else {
			klog.Fatalf("no ClusterID found.  A ClusterID is required for the cloud provider to function properly.  This check can be bypassed by setting the allow-untagged-cloud option")
		}
	}
	return cloud
}
