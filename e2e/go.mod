module e2e_test

go 1.22

toolchain go1.22.2

require (
	github.com/appscode/go v0.0.0-20200323182826-54e98e09185a
	github.com/linode/linodego v1.34.0
	github.com/onsi/ginkgo/v2 v2.17.1
	github.com/onsi/gomega v1.33.0
	k8s.io/api v0.23.17
	k8s.io/apimachinery v0.23.17
	k8s.io/client-go v1.5.2
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-resty/resty/v2 v2.13.1 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/gofuzz v1.1.0 // indirect
	github.com/google/pprof v0.0.0-20210407192527-94a9f03dee38 // indirect
	github.com/googleapis/gnostic v0.5.5 // indirect
	github.com/imdario/mergo v0.3.5 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	golang.org/x/net v0.25.0 // indirect
	golang.org/x/oauth2 v0.20.0 // indirect
	golang.org/x/sys v0.20.0 // indirect
	golang.org/x/term v0.20.0 // indirect
	golang.org/x/text v0.15.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	golang.org/x/tools v0.17.0 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/klog/v2 v2.30.0 // indirect
	k8s.io/kube-openapi v0.0.0-20211115234752-e816edb12b65 // indirect
	k8s.io/utils v0.0.0-20211116205334-6203023598ed // indirect
	sigs.k8s.io/json v0.0.0-20211020170558-c049b76a60c6 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
	sigs.k8s.io/yaml v1.2.0 // indirect
)

replace (
	k8s.io/api => k8s.io/api v0.23.17
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.23.17
	k8s.io/apimachinery => k8s.io/apimachinery v0.23.17
	k8s.io/apiserver => k8s.io/apiserver v0.23.17
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.23.17
	k8s.io/client-go => k8s.io/client-go v0.23.17
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.23.17
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.23.17
	k8s.io/code-generator => k8s.io/code-generator v0.23.17
	k8s.io/component-base => k8s.io/component-base v0.23.17
	k8s.io/cri-api => k8s.io/cri-api v0.23.17
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.23.17
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.23.17
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.23.17
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.23.17
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.23.17
	k8s.io/kubectl => k8s.io/kubectl v0.23.17
	k8s.io/kubelet => k8s.io/kubelet v0.23.17
	k8s.io/kubernetes => k8s.io/kubernetes v0.23.17
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.23.17
	k8s.io/metrics => k8s.io/metrics v0.23.17
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.23.17
)
