FROM golang:1.24-alpine AS builder
RUN mkdir -p /linode
WORKDIR /linode

COPY go.mod .
COPY go.sum .
COPY main.go .
COPY cloud ./cloud
COPY sentry ./sentry

RUN go mod download
RUN go build -a -ldflags '-extldflags "-static"' -o /bin/linode-cloud-controller-manager-linux /linode

FROM alpine:3.22.0
RUN apk add --update --no-cache ca-certificates
LABEL maintainers="Linode"
LABEL description="Linode Cloud Controller Manager"
COPY --from=builder /bin/linode-cloud-controller-manager-linux /linode-cloud-controller-manager-linux
ENTRYPOINT ["/linode-cloud-controller-manager-linux"]
