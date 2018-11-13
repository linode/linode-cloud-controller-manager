## Development Guide
This document is intended to be the canonical source of truth for things like
supported toolchain versions for building the Linode Cloud Controller
Manager. If you find a requirement that this doc does not capture, please
submit an issue on github.

This document is intended to be relative to the branch in which it is found.
It is guaranteed that requirements will change over time for the development
branch, but release branches of the Linode Cloud Controller Manager should
not change.

### Building the Linode Cloud Controller Manager
Some of the Linode Cloud Controller Manager development helper scripts rely
on a fairly up-to-date GNU tools environment, so most recent Linux distros
should work just fine out-of-the-box.

#### Setup Go
The Linode Cloud Controller Manager is written in Google's Go programming
language. Currently, the Linode Cloud Controller Manager is developed and
tested on **Go 1.8.3**. If you haven't set up a Go development environment,
please follow [these instructions](https://golang.org/doc/code.html) to
install Go.

#### Download Source

```console
$ go get github.com/linode/linode-cloud-controller-manager
$ cd $(go env GOPATH)/src/github.com/linode/linode-cloud-controller-manager
```

#### Install Dev tools
To install various dev tools for Pharm Controller Manager, run the following command:

```console
# setting up dependencies for compiling cloud-controller-manager...
$ ./hack/builddeps.sh
```

#### Build Binary
The build script currently supports only Python 2. Refer to your distribution
docs on how to install Python 2 and Python 2 packages. If you're on macOS:

```bash
brew install python2
pip2 install git+https://github.com/ellisonbg/antipackage.git#egg=antipackage
```

```
$ ./hack/make.py
$ linode-cloud-controller-manager version
```

#### Dependency management
Linode Cloud Controller Manager uses
[Glide](https://github.com/Masterminds/glide) to manage dependencies.
Dependencies are already checked in the `vendor` folder. If you want to
update/add dependencies, run:
```console
$ glide slow
```

#### Build Docker images
To build and push your custom Docker image, follow the steps below. To
release a new version of Linode Cloud Controller Manager, please follow the
[release guide](/docs/developer-guide/release.md).

```console
# Build Docker image
$ ./hack/docker/setup.sh; ./hack/docker/setup.sh push

# Add docker tag for your repository
$ docker tag linode/linode-cloud-controller-manager:<tag> <image>:<tag>

# Push Image
$ docker push <image>:<tag>
```

#### Generate CLI Reference Docs
```console
$ ./hack/gendocs/make.sh
```
