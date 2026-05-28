# syntax=docker/dockerfile:1.7

FROM debian:trixie-slim AS builder

ARG DEBIAN_FRONTEND=noninteractive
ARG HSFLOWD_REF=v2.1.26-1

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      ca-certificates \
      git \
      build-essential \
      make \
      gcc \
      clang \
      pkg-config \
      libnfnetlink-dev \
      libdbus-1-dev \
      libsystemd-dev \
      uuid-dev \
      libssl-dev \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /src

RUN git clone --depth 1 --branch ${HSFLOWD_REF} https://github.com/sflow/host-sflow.git

WORKDIR /src/host-sflow

RUN set -eux; \
    make -C src/sflow; \
    make -C src/json; \
    HSFLOWD_VERSION="$(sh ./getVersion)"; \
    make -C src/Linux VERSION="${HSFLOWD_VERSION}" FEATURES=CUMULUS; \
    make -C src/Linux VERSION="${HSFLOWD_VERSION}" INSTROOT=/out FEATURES=CUMULUS install


FROM debian:trixie-slim

ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      ca-certificates \
      iproute2 \
      procps \
      net-tools \
      libnfnetlink0 \
      libdbus-1-3 \
      libsystemd0 \
      libuuid1 \
      libssl3 \
 && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/usr/ /usr/
COPY --from=builder /out/etc/ /etc/
COPY --from=builder /out/lib/systemd/system/ /usr/lib/systemd/system/
COPY image/hsflowd/hsflowd.conf /etc/hsflowd.conf

RUN chmod +x /usr/sbin/hsflowd

ENTRYPOINT ["/usr/sbin/hsflowd"]
CMD ["-c", "/var/log/hsflowd.crash"]
