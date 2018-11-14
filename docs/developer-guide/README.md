## Development Guide
This document is intended to be the canonical source of truth for things like
supported toolchain versions for building the Linode Cloud Controller
Manager. If you find a requirement that this doc does not capture, please
submit an issue on GitHub.

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
please follow [these instructions](https://golang.org/doc/install) to
install Go.

On macOS, Homebrew has a nice package

```bash
brew install golang
```

#### Download Source

```bash
$ go get github.com/linode/linode-cloud-controller-manager
$ cd $(go env GOPATH)/src/github.com/linode/linode-cloud-controller-manager
```

#### Install Dev tools
To install various dev tools for Pharm Controller Manager, run the following command:

```console
$ ./hack/builddeps.sh
```

#### Build Binary
Use the following Make targets to build and run a local binary

```
$ make build
$ make run
# You can also run the binary that was built to pass additional args
$ dist/linode-cloud-controller-manager run
```

#### Dependency management
Linode Cloud Controller Manager uses
[Glide](https://github.com/Masterminds/glide) to manage dependencies.
Dependencies are already checked in the `vendor` folder. If you want to
update/add dependencies, run:
```console
$ glide slow
```

#### Building Docker images
To build and push a Docker image, use the following make targets. 

```bash
# Set the repo/image:tag with the TAG environment variable
# Then run the docker-build make target
$ IMG=linode/linode-cloud-controller-manager:canary make docker-build

# Push Image
$ IMG=linode/linode-cloud-controller-manager:canary make docker-push
```

Then, to run the image
```bash
$ docker run -ti linode/linode-cloud-controller-manager:canary
```
