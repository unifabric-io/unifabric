# syntax=docker/dockerfile:1.7

FROM golang:1.26.2 AS builder
ARG TARGETARCH
WORKDIR /unifabric
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o /unifabric/agent cmd/agent/main.go

FROM ubuntu:24.04
RUN apt-get update && apt-get install -y lldpd iproute2
WORKDIR /unifabric
RUN mkdir -p /usr/bin/unifabric
COPY --from=builder /unifabric/agent /usr/bin/unifabric/agent
COPY image/agent/entrypoint.sh /usr/bin/unifabric/entrypoint.sh
RUN chmod +x /usr/bin/unifabric/entrypoint.sh
RUN chmod +x /usr/bin/unifabric/agent
RUN date -u '+%Y-%m-%d %H:%M:%S' > VERSION
