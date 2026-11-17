# syntax=docker/dockerfile:1.7
#
# Kinora – multi-stage build.
#   dev     : Go toolchain + ffmpeg, used by scripts/go.sh for vet/test without a local Go install
#   build   : compiles the cgo (sqlite) binary
#   runtime : debian:bookworm-slim + Debian ffmpeg + VA-API drivers, non-root "kinora" user
ARG GO_VERSION=1.24

FROM golang:${GO_VERSION}-bookworm AS dev
RUN apt-get update \
 && apt-get install -y --no-install-recommends ffmpeg libchromaprint-tools \
 && rm -rf /var/lib/apt/lists/*
WORKDIR /src

FROM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 go build -trimpath \
      -ldflags "-s -w -X github.com/ScotyLucas/kinora/internal/server.Version=${VERSION}" \
      -o /out/kinora ./cmd/kinora

FROM debian:bookworm-slim AS runtime
ARG TARGETARCH
ARG VERSION=dev
# AMD=1 adds Mesa's VA-API driver (radeonsi) for AMD GPUs. It pulls in LLVM (~115 MB),
# so the default image leaves it out: Intel VA-API/QSV, NVIDIA NVENC and software encoding work without it.
ARG AMD=0
# Debian's own ffmpeg (5.1, built with libzimg/zscale, VA-API, QSV via libmfx/vpl, NVENC via ffnvcodec).
# libchromaprint-tools adds fpcalc, the fallback fingerprinter of the Skip Intro detector.
# non-free component is enabled only for intel-media-va-driver-non-free (amd64, Gen8+ Intel iGPU incl. QSV).
RUN set -eux; \
    sed -i 's/^Components: main$/Components: main non-free non-free-firmware/' /etc/apt/sources.list.d/debian.sources; \
    apt-get update; \
    extra=""; \
    if [ "$TARGETARCH" = "amd64" ]; then extra="intel-media-va-driver-non-free"; fi; \
    if [ "$AMD" = "1" ]; then extra="$extra mesa-va-drivers"; fi; \
    apt-get install -y --no-install-recommends ffmpeg libchromaprint-tools ca-certificates tzdata gosu $extra; \
    if [ "$AMD" != "1" ]; then \
      # libavdevice -> libgl1 -> libglx-mesa0 -> libgl1-mesa-dri -> libllvm15 (+ libz3). The ffmpeg CLI never
      # creates an OpenGL context, so the Mesa DRI drivers (the only LLVM users) are dead weight here.
      # Removed with --force-depends: apt inside this image will report the gap, which is fine for an appliance.
      dpkg --purge --force-depends libgl1-mesa-dri libllvm15 libz3-4; \
      ffmpeg -hide_banner -f lavfi -i testsrc2=size=320x240:rate=5 -t 1 -c:v libx264 -f null - ; \
    fi; \
    rm -rf /var/lib/apt/lists/*; \
    groupadd -g 1000 kinora; \
    useradd -u 1000 -g 1000 -d /config -s /usr/sbin/nologin -M kinora; \
    mkdir -p /config /cache /media; \
    chown kinora:kinora /config /cache

COPY --from=build /out/kinora /usr/local/bin/kinora
COPY docker/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod 0755 /usr/local/bin/entrypoint.sh /usr/local/bin/kinora

ENV KINORA_INSTALL=docker \
    KINORA_ADDR=:8096 \
    KINORA_DATA=/config \
    KINORA_CACHE=/cache \
    KINORA_MEDIA_ROOT=/media \
    PUID=1000 \
    PGID=1000

LABEL org.opencontainers.image.title="Kinora" \
      org.opencontainers.image.description="Self-hosted media server – a friendlier Jellyfin alternative" \
      org.opencontainers.image.source="https://github.com/ScotyLucas/kinora" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.licenses="MIT"

EXPOSE 8096
VOLUME ["/config", "/cache"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD ["/usr/local/bin/kinora", "healthcheck"]
STOPSIGNAL SIGTERM
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["/usr/local/bin/kinora"]
