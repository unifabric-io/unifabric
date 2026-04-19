# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM golang:1.26.2 AS builder
ARG TARGETARCH
ARG TARGETOS
WORKDIR /unifabric
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /unifabric/agent cmd/agent/main.go
RUN date -u '+%Y-%m-%d %H:%M:%S' > /unifabric/VERSION

FROM ubuntu:24.04
RUN apt-get update && apt-get install -y lldpd iproute2
WORKDIR /unifabric
COPY --from=builder /unifabric/agent /usr/bin/unifabric/agent
COPY --from=builder /unifabric/VERSION VERSION
COPY --chmod=755 image/agent/entrypoint.sh /usr/bin/unifabric/entrypoint.sh
