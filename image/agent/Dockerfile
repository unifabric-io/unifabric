# syntax=docker/dockerfile:1.7

# bpf compiles the eBPF flow program. It runs on the target platform rather
# than the build platform because the program includes asm/ptrace.h, whose
# struct pt_regs differs per architecture and comes from the native
# linux-libc-dev headers. It is a single small file, so emulation is cheap.
FROM ubuntu:24.04 AS bpf
ARG TARGETARCH
RUN apt-get update && apt-get install -y --no-install-recommends clang libbpf-dev dpkg-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /unifabric
COPY pkg/agent/ebpfflow/bpf/rdma_flow.bpf.c bpf/
# amd64 maps to the x86 BPF target arch, arm64 keeps its name.
RUN BPF_ARCH=$(echo "${TARGETARCH}" | sed 's/^amd64$/x86/') \
    && clang -O2 -g -Wall -Werror -target bpf -D__TARGET_ARCH_${BPF_ARCH} \
       -I/usr/include/$(dpkg-architecture -qDEB_HOST_MULTIARCH) \
       -c bpf/rdma_flow.bpf.c -o /unifabric/rdma_flow.bpf.o

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
COPY --from=bpf /unifabric/rdma_flow.bpf.o /usr/lib/unifabric/rdma_flow.bpf.o
COPY --chmod=755 image/agent/entrypoint.sh /usr/bin/unifabric/entrypoint.sh
