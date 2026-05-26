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

FROM alpine:3.18 AS lldpcli-1-0-4
ARG LLDPD_104_VERSION=1.0.4
RUN apk add --no-cache \
    autoconf \
    automake \
    build-base \
    curl \
    json-c-dev \
    libevent-dev \
    libtool \
    libxml2-dev \
    linux-headers \
    readline-dev
WORKDIR /src
RUN curl -fsSL "https://github.com/lldpd/lldpd/archive/refs/tags/${LLDPD_104_VERSION}.tar.gz" \
    | tar -xz --strip-components=1
RUN echo "${LLDPD_104_VERSION}" > .dist-version
RUN ./autogen.sh \
    && ./configure \
        --prefix=/usr/local \
        --localstatedir=/var \
        --disable-static \
        --without-snmp \
        --without-seccomp \
    && make -j"$(nproc)" \
    && make install DESTDIR=/out

FROM alpine:3.18 AS lldpcli-1-0-16
RUN apk add --no-cache 'lldpd=1.0.16-r1'

FROM alpine:3.22.4
RUN apk add --no-cache json-c libbsd libevent libxml2 readline util-linux
WORKDIR /unifabric
COPY --from=builder /unifabric/switch-agent /usr/bin/unifabric/switch-agent
COPY --from=builder /unifabric/VERSION VERSION
COPY --from=lldpcli-1-0-4 /out/usr/local/sbin/lldpcli /opt/lldpcli/1.0.4/lldpcli
COPY --from=lldpcli-1-0-4 /out/usr/local/lib/liblldpctl.so.4* /opt/lldpcli/1.0.4/lib/
COPY --from=lldpcli-1-0-16 /usr/sbin/lldpcli /opt/lldpcli/1.0.16/lldpcli
COPY --from=lldpcli-1-0-16 /usr/lib/liblldpctl.so.4* /opt/lldpcli/1.0.16/lib/
RUN mkdir -p /usr/local/sbin \
    && printf '#!/bin/sh\nexport LD_LIBRARY_PATH=/opt/lldpcli/1.0.4/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}\nexec /opt/lldpcli/1.0.4/lldpcli "$@"\n' > /usr/local/sbin/lldpcli-1.0.4 \
    && printf '#!/bin/sh\nexport LD_LIBRARY_PATH=/opt/lldpcli/1.0.16/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}\nexec /opt/lldpcli/1.0.16/lldpcli "$@"\n' > /usr/local/sbin/lldpcli-1.0.16 \
    && chmod +x /usr/local/sbin/lldpcli-1.0.4 /usr/local/sbin/lldpcli-1.0.16 \
    && ln -s /usr/local/sbin/lldpcli-1.0.16 /usr/local/sbin/lldpcli \
    && lldpcli-1.0.4 -v \
    && lldpcli-1.0.16 -v
