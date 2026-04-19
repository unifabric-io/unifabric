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
    CGO_ENABLED=0 GOOS=linux go build -o /unifabric/controller cmd/controller/main.go


FROM alpine:3.22.4
WORKDIR /unifabric
RUN mkdir -p /usr/bin/unifabric
COPY --from=builder /unifabric/controller /usr/bin/unifabric/controller
RUN date -u '+%Y-%m-%d %H:%M:%S' > VERSION
