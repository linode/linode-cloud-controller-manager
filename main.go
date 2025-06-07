package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"

	"github.com/linode/linodego"
	"github.com/spf13/pflag"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/cloud-provider/app"
	"k8s.io/cloud-provider/app/config"
	"k8s.io/cloud-provider/names"
	"k8s.io/cloud-provider/options"
	utilflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	"k8s.io/klog/v2"

	"github.com/linode/linode-cloud-controller-manager/cloud/linode"
	"github.com/linode/linode-cloud-controller-manager/sentry"

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
		klog.Errorf("%s not set, not initializing Sentry\n", sentryDSNVariable)
		return
	}

	if environment, ok = os.LookupEnv(sentryEnvironmentVariable); !ok {
		klog.Errorf("%s not set, not initializing Sentry\n", sentryEnvironmentVariable)
		return
	}

	if release, ok = os.LookupEnv(sentryReleaseVariable); !ok {
		klog.Infof("%s not set, defaulting to unknown", sentryReleaseVariable)
		release = "unknown"
	}

	if err := sentry.Initialize(dsn, environment, release); err != nil {
		klog.Errorf("error initializing sentry: %s\n", err.Error())
		return
	}

	klog.Infoln("Sentry successfully initialized")
}

func main() {
	klog.Infoln("Linode Cloud Controller Manager starting up")

	initializeSentry()

	ctx := sentry.SetHubOnContext(context.Background())

	ccmOptions, err := options.NewCloudControllerManagerOptions()
	if err != nil {
		klog.Fatalf("unable to initialize command options: %v", err)
	}
	fss := utilflag.NamedFlagSets{}
	controllerAliases := names.CCMControllerAliases()
	stopCh := make(chan struct{})
	command := app.NewCloudControllerManagerCommand(ccmOptions, cloudInitializer, app.DefaultInitFuncConstructors, controllerAliases, fss, stopCh)

	// Add Linode-specific flags
	command.Flags().BoolVar(&linode.Options.LinodeGoDebug, "linodego-debug", false, "enables debug output for the LinodeAPI wrapper")
	command.Flags().BoolVar(&linode.Options.EnableRouteController, "enable-route-controller", false, "enables route_controller for ccm")
	command.Flags().BoolVar(&linode.Options.EnableTokenHealthChecker, "enable-token-health-checker", false, "enables Linode API token health checker")
	command.Flags().StringVar(&linode.Options.VPCName, "vpc-name", "", "[deprecated: use vpc-names instead] vpc name whose routes will be managed by route-controller")
	command.Flags().StringVar(&linode.Options.VPCNames, "vpc-names", "", "comma separated vpc names whose routes will be managed by route-controller")
	command.Flags().StringVar(&linode.Options.SubnetNames, "subnet-names", "default", "comma separated subnet names whose routes will be managed by route-controller (requires vpc-names flag to also be set)")
	command.Flags().StringVar(&linode.Options.LoadBalancerType, "load-balancer-type", "nodebalancer", "configures which type of load-balancing to use for LoadBalancer Services (options: nodebalancer, cilium-bgp)")
	command.Flags().StringVar(&linode.Options.BGPNodeSelector, "bgp-node-selector", "", "node selector to use to perform shared IP fail-over with BGP (e.g. cilium-bgp-peering=true")
	command.Flags().StringVar(&linode.Options.IpHolderSuffix, "ip-holder-suffix", "", "suffix to append to the ip holder name when using shared IP fail-over with BGP (e.g. ip-holder-suffix=my-cluster-name")
	command.Flags().StringVar(&linode.Options.DefaultNBType, "default-nodebalancer-type", string(linodego.NBTypeCommon), "default type of NodeBalancer to create (options: common, premium)")
	command.Flags().StringVar(&linode.Options.NodeBalancerBackendIPv4Subnet, "nodebalancer-backend-ipv4-subnet", "", "ipv4 subnet to use for NodeBalancer backends")
	command.Flags().StringSliceVar(&linode.Options.NodeBalancerTags, "nodebalancer-tags", []string{}, "Linode tags to apply to all NodeBalancers")
	command.Flags().BoolVar(&linode.Options.EnableIPv6ForLoadBalancers, "enable-ipv6-for-loadbalancers", false, "set both IPv4 and IPv6 addresses for all LoadBalancer services (when disabled, only IPv4 is used)")
	command.Flags().IntVar(&linode.Options.NodeCIDRMaskSizeIPv4, "node-cidr-mask-size-ipv4", 0, "ipv4 cidr mask size for pod cidrs allocated to nodes")
	command.Flags().IntVar(&linode.Options.NodeCIDRMaskSizeIPv6, "node-cidr-mask-size-ipv6", 0, "ipv6 cidr mask size for pod cidrs allocated to nodes")
	command.Flags().IntVar(&linode.Options.NodeBalancerBackendIPv4SubnetID, "nodebalancer-backend-ipv4-subnet-id", 0, "ipv4 subnet id to use for NodeBalancer backends")
	command.Flags().StringVar(&linode.Options.NodeBalancerBackendIPv4SubnetName, "nodebalancer-backend-ipv4-subnet-name", "", "ipv4 subnet name to use for NodeBalancer backends")
	command.Flags().BoolVar(&linode.Options.DisableNodeBalancerVPCBackends, "disable-nodebalancer-vpc-backends", false, "disables nodebalancer backends in VPCs (when enabled, nodebalancers will only have private IPs as backends for backward compatibility)")

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
		sentry.CaptureError(ctx, fmt.Errorf("%s", msg))
		fmt.Fprintf(os.Stderr, "kubeconfig missing from CCM flag set"+"\n")
		os.Exit(1)
	}

	if externalSubnet, ok := os.LookupEnv(linodeExternalSubnetVariable); ok && externalSubnet != "" {
		_, network, err := net.ParseCIDR(externalSubnet)
		if err != nil {
			msg := fmt.Sprintf("Unable to parse %s as network subnet: %v", externalSubnet, err)
			sentry.CaptureError(ctx, fmt.Errorf("%s", msg))
			fmt.Fprintf(os.Stderr, "%v\n", msg)
			os.Exit(1)
		}
		linode.Options.LinodeExternalNetwork = network
	}

	// Provide stop channel for linode authenticated client healthchecker
	linode.Options.GlobalStopChannel = stopCh

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
	if config.ComponentConfig.KubeCloudShared.AllocateNodeCIDRs {
		linode.Options.AllocateNodeCIDRs = true
		if config.ComponentConfig.KubeCloudShared.ClusterCIDR == "" {
			fmt.Fprintf(os.Stderr, "--cluster-cidr is not set. This is required if --allocate-node-cidrs is set.\n")
			os.Exit(1)
		}
		linode.Options.ClusterCIDRIPv4 = config.ComponentConfig.KubeCloudShared.ClusterCIDR
	}
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
