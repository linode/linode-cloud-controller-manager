# "docker": build in a Docker build container (default)
# "local":  copy from the build context. This is useful for tilt's live_update
#           feature, allowing hot reload of the linode-ccm binary for fast
#           development iteration. See ./Tiltfile
ARG BUILD_SOURCE="docker"

FROM golang:1.21-alpine as builder
RUN mkdir -p /linode
WORKDIR /linode

COPY go.mod .
COPY go.sum .
COPY main.go .
COPY cloud ./cloud
COPY sentry ./sentry

RUN go mod download
RUN go build -a -ldflags '-extldflags "-static"' -o /bin/linode-cloud-controller-manager-linux /linode

FROM alpine:3.18.4
RUN apk add --update --no-cache ca-certificates
LABEL maintainers="Linode"
LABEL description="Linode Cloud Controller Manager"
COPY --from=builder /bin/linode-cloud-controller-manager-linux /linode-cloud-controller-manager-linux
ENTRYPOINT ["/linode-cloud-controller-manager-linux"]
