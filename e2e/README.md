## How to run these End-to-end (e2e) tests

Install the following packages (macOS examples)

```
brew install terraform
brew install kubectl
brew install hg
brew install golang
```

Add the following environment variables to your shell rc

```
export LINODE_API_TOKEN=<your linode API token>

export GOPATH=$HOME/go
export PATH=$HOME/go/bin:$PATH
export GO111MODULE=on 
```

If you need a Linode API token visit this page:
https://cloud.linode.com/profile/tokens

Then, `go get` this repo
`go get github.com/linode/linode-cloud-controller-manager`

That may fail, if it does, navigate to the directory that was created and run `go mod tidy`:

```
cd ~/go/src/github.com/linode/linode-cloud-controller-manager
go mod tidy
```

Then, use the makefile in the directory above this directory to build the CCM (this is to download goimports)

```
cd $GOPATH/src/github.com/linode/linode-cloud-controller-manager
make build
```

By default the tests use $HOME/.ssh/id\_rsa.pub as the public key used to provision the cluster, so it needs to be added to your agent.

```
ssh-add $HOME/.ssh/id_rsa
```

Come back here and run the tests

```
cd e2e
make test
```

To save time on multiple runs by allowing the cluster to remain, use `make reuse-and-test`

### Generating a new server certificate for testing

Some of the tests require a secret containing a TLS certificate, for use in creating or updating a NodeBalancer config using TLS. A CA certificate and server certificate can be found in the `test/certificates` directory. The server certificate, used for TLS NodeBalancer configs, has an expiry of 4 years. You can use the following command to generate a new TLS certificate, using the existing CSR:

```
openssl x509 -req -in test/certificates/server.csr -CA test/certificates/ca.crt -CAkey test/certificates/ca.key -CAcreateserial -out test/certificates/server.crt -days 1440 -sha256 -extfile <(printf "subjectAltName=DNS:linode.test,DNS:www.linode.test")
```
Once a new cert is generated, you will need to replace the existing constants, "serverCert" and "serverKey", in test/framework/secret.go.
