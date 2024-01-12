# "docker": build in a Docker build container (default)
# "local":  copy from the build context. This is useful for tilt's live_update
#           feature, allowing hot reload of the kpp-api binary for fast
#           development iteration. See ./Tiltfile
ARG BUILD_SOURCE="docker"

FROM golang:1.21-alpine as builder
RUN apk add --update --no-cache make
RUN mkdir -p /linode
WORKDIR /linode

COPY go.mod .
COPY go.sum .
COPY main.go .
COPY cloud ./cloud
COPY sentry ./sentry
COPY Makefile .

RUN make build-linux

FROM alpine:3.18.4 as base
RUN apk add --update --no-cache ca-certificates
LABEL maintainers="Linode"
LABEL description="Linode Cloud Controller Manager"

# Copy from docker
FROM base as docker
COPY --from=builder /linode/dist/linode-cloud-controller-manager-linux-amd64 /linode-cloud-controller-manager-linux

# Copy from local
FROM base as local
COPY dist/linode-cloud-controller-manager-linux-amd64 /linode-cloud-controller-manager-linux

# See documentation on the arg
FROM $BUILD_SOURCE
ENTRYPOINT ["/linode-cloud-controller-manager-linux"]
