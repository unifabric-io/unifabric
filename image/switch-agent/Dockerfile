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
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /unifabric/switch-agent cmd/switch-agent/main.go
RUN date -u '+%Y-%m-%d %H:%M:%S' > /unifabric/VERSION

FROM alpine:3.22.4
RUN apk add --no-cache util-linux
WORKDIR /unifabric
COPY --from=builder /unifabric/switch-agent /usr/bin/unifabric/switch-agent
COPY --from=builder /unifabric/VERSION VERSION