load("ext://deployment", "deployment_create")
load("ext://k8s_attach", "k8s_attach")
load("ext://restart_process", "docker_build_with_restart")
load("ext://helm_resource", "helm_resource", "helm_repo")

allow_k8s_contexts(k8s_context())
# helm_repo("registry-repo", "https://helm.twun.io", labels=["helm-repos"])

# # in order to use this registry you must add `127.0.0.1      registry-proxy` in your /etc/hosts
# # and add http://registry-proxy:5000 to the list of insecure registries in docker daemon.
load("ext://k8s_attach", "k8s_attach")

# # Deploy a docker egistry to the cluster, this registry will be used for hosting images built by tilt.
# helm_resource(
#     "docker-registry",
#     "registry-repo/docker-registry",
#     namespace="registry",
#     flags=[
#         "--create-namespace",
#         "--version=2.2.2",
#         "--set=metrics.enabled=true",
#         "--set=storage=filesystem",
#         #This [documentation](https://docs.docker.com/registry/configuration/#debug) gives more detail about the debug endpoint and how it can be used in production
#         #Disabling this will hide the registry metrics on the 5001 port
#     ],
#     labels=["registry"],
#     resource_deps=["registry-repo"],
# )

local_resource(
    "registry-probe",
    serve_cmd="sleep infinity",
    readiness_probe=probe(
        period_secs=15,
        http_get=http_get_action(host="registry-proxy", port=5000, path="/"),
    ),
    labels=["registry"],
    #resource_deps=["docker-registry"],
)

k8s_attach(
    "registry-port-forward",
    "deployment/docker-registry",
    namespace="registry",
    port_forwards=[5000],
    labels=["registry"],
)
default_registry("registry-proxy:5000", host_from_cluster="docker.local")

labels = ["ccm-linode"]

local_resource(
    "linode-cloud-controller-manager-linux-amd64",
    "make build-linux",
    # No glob support :(
    deps=[
        "cloud/",
        "go.mod",
        "go.sum",
        "main.go",
        "Makefile",
        "sentry/",
    ],
    ignore=[
        "**/*.bin",
        "**/gomock*",
        "cloud/linode/mock_client_test.go",
    ],
    labels=labels,
)

entrypoint = "/linode-cloud-controller-manager-linux"
docker_build_with_restart(
    "linode/linode-cloud-controller-manager",
    ".",
    entrypoint=[entrypoint],
    ignore=["**/*.go", "go.*", "Makefile"],
    live_update=[
        sync(
            "dist/linode-cloud-controller-manager-linux-amd64",
            entrypoint,
        ),
    ],
    build_args={"BUILD_SOURCE": "local"},
    platform="linux/amd64",
)

chart_yaml = helm(
    "deploy/chart",
    name="ccm-linode",
    namespace="kube-system",
    values="./tilt.values.yaml",
)
k8s_yaml(chart_yaml)
