# Build the manager binary
FROM golang:1.12.4 as builder

WORKDIR /go/src/github.com/linode/linode-cloud-controller-manager
COPY go.* *.go ./
COPY cloud/ ./cloud
ENV GO111MODULE=on

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o linode-cloud-controller-manager github.com/linode/linode-cloud-controller-manager

FROM alpine:latest
RUN apk update && apk add ca-certificates && rm -rf /var/cache/apk/*
WORKDIR /root/
COPY --from=builder /go/src/github.com/linode/linode-cloud-controller-manager/linode-cloud-controller-manager /
ENTRYPOINT ["/linode-cloud-controller-manager"]
