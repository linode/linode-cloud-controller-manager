package main

import (
	"context"
	goflag "flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/linode/linode-cloud-controller-manager/cloud/linode"
	"github.com/linode/linode-cloud-controller-manager/sentry"
	"github.com/spf13/pflag"
	utilflag "k8s.io/apiserver/pkg/util/flag"
	"k8s.io/apiserver/pkg/util/logs"
	"k8s.io/kubernetes/cmd/cloud-controller-manager/app"
	_ "k8s.io/kubernetes/pkg/client/metrics/prometheus" // for client metric registration
	_ "k8s.io/kubernetes/pkg/version/prometheus"        // for version metric registration
)

const (
	sentryDSNVariable         = "SENTRY_DSN"
	sentryEnvironmentVariable = "SENTRY_ENVIRONMENT"
	sentryReleaseVariable     = "SENTRY_RELEASE"
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

	rand.Seed(time.Now().UTC().UnixNano())

	command := app.NewCloudControllerManagerCommand()

	// Add Linode-specific flags
	command.Flags().BoolVar(&linode.Options.LinodeGoDebug, "linodego-debug", false, "enables debug output for the LinodeAPI wrapper")

	// Make the Linode-specific CCM bits aware of the kubeconfig flag
	linode.Options.KubeconfigFlag = command.Flags().Lookup("kubeconfig")
	if linode.Options.KubeconfigFlag == nil {
		msg := "kubeconfig missing from CCM flag set"
		sentry.CaptureError(ctx, fmt.Errorf(msg))
		fmt.Fprintf(os.Stderr, "kubeconfig missing from CCM flag set"+"\n")
		os.Exit(1)
	}

	pflag.CommandLine.SetNormalizeFunc(utilflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(goflag.CommandLine)

	logs.InitLogs()
	defer logs.FlushLogs()

	if err := command.Execute(); err != nil {
		sentry.CaptureError(ctx, err)
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
