# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

<!--- Add changes here before release, then move them when a release is cut --->

### Added

* New LoadBalancer TLS annotations.

example:

```
service.beta.kubernetes.io/linode-loadbalancer-default-protocol: "http"
service.beta.kubernetes.io/linode-loadbalancer-port-443: |
    {
        "tls-secret-name": "prod-app-tls",
        "protocol": "https"
    }
```

### Fixed

* New syntax fixes an issue where a creating a load balancer created with both
  an http and https port would fail silently.
* Some error messages changed to meet linter standards

### Deprecated

* Former annotations `linode-loadbalancer-tls` and `linode-loadbalancer-protocol` will
  be removed Q3 2020.

# Release History

## [v0.2.4] (2019-10-03)

### Enhancements

* Dependencies updated.

## [v0.2.3] (2019-06-26)

### Features

* Support for setting root CA cert (linodego 0.10.0).

### Enhancements

* Binary is now cross-compiled locally for faster container builds.
* Makefile cleaned up for saner prereqs and ELF vs. local builds.

## [v0.2.2] (2019-05-29)

### Features

* Upgrade linodego to version 0.9.0 for various new API features.

## [v0.2.1] (2019-04-16)

### Features

* Support for LoadBalancer TLS annotations.

example:

```
service.beta.kubernetes.io/linode-loadbalancer-tls: "[ { "tls-secret-name": "prod-app-tls", "port": 443}, {"tls-secret-name": "dev-app-tls", "port": 8443} ]"
```

