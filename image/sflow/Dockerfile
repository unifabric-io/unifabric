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
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /unifabric/sflow cmd/sflow/main.go
RUN date -u '+%Y-%m-%d %H:%M:%S' > /unifabric/VERSION

FROM alpine:3.22.4
WORKDIR /unifabric
COPY --from=builder /unifabric/sflow /usr/bin/unifabric/sflow
COPY --from=builder /unifabric/VERSION VERSION
