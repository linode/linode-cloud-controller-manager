# Release History

## [v0.2.3](https://github.com/linode/linode-cloud-controller-manager/compare/v0.2.2..v0.2.3) (2019-06-26)

### Features

* Support for setting root CA cert (linodego 0.10.0)

### Enhancements

* Binary is now cross-compiled locally for faster container builds
* Makefile cleaned up for saner prereqs and ELF vs. local builds

## [v0.2.2](https://github.com/linode/linode-cloud-controller-manager/compare/v0.2.1..v0.2.2) (2019-05-29)

### Features

* Upgrade linodego to version 0.9.0 for various new API features.

## [v0.2.1](https://github.com/linode/linode-cloud-controller-manager/tree/v0.2.1) (2019-04-16)

### Features

* Support for LoadBalancer TLS annotations

example:

```
service.beta.kubernetes.io/linode-loadbalancer-tls: "[ { "tls-secret-name": "prod-app-tls", "port": 443}, {"tls-secret-name": "dev-app-tls", "port": 8443} ]"
```

