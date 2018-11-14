# Build the manager binary
FROM golang:1.11.2 as builder

WORKDIR /go/src/github.com/linode/linode-cloud-controller-manager
COPY cloud/    cloud/
COPY cmds/     cmds/
COPY *.go      ./
COPY vendor/   vendor/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o linode-cloud-controller-manager github.com/linode/linode-cloud-controller-manager

FROM ubuntu:latest
WORKDIR /root/
COPY --from=builder /go/src/github.com/linode/linode-cloud-controller-manager/linode-cloud-controller-manager .
ENTRYPOINT ["./linode-cloud-controller-manager"]