# Node IPAM using CCM

## Overview

Linode CCM supports configuring and managing pod CIDRs allocated to nodes. This includes both ipv4 and ipv6 CIDRs. It is disabled by default. It can be enabled by starting CCM with `--allocate-node-cidrs` and `--cluster-cidr` flags.

```yaml
spec:
  template:
    spec:
      containers:
        - name: ccm-linode
          args:
            - --allocate-node-cidrs=true
            - --cluster-cidr=10.192.0.0/10
```

Once enabled, CCM will manage and allocate pod CIDRs to nodes.

Note:
Make sure node IPAM allocation is disabled in kube-controller-manager to avoid both controllers competing to assign CIDRs to nodes. To make sure its disabled, check and make sure kube-controller-manager is not started with `--allocate-node-cidrs` flag.

## Allocated subnet size
By default, CCM allocates /24 subnet for ipv4 addresses and /64 for ipv6 addresses to nodes. If one wants different subnet range, it can be configured by using `--node-cidr-mask-size-ipv4` and `--node-cidr-mask-size-ipv6` flags.

```yaml
spec:
  template:
    spec:
      containers:
        - name: ccm-linode
          args:
            - --allocate-node-cidrs=true
            - --cluster-cidr=10.192.0.0/10,fd00::/56
            - --node-cidr-mask-size-ipv4=25
            - --node-cidr-mask-size-ipv6=64
```
