# Tabernacle builds to a single static binary, so the runtime image needs no
# toolchain, no C library and no network access.
FROM golang:1.27-alpine AS build

WORKDIR /src

# Dependencies first, so a source-only change reuses the module layer.
COPY go.mod go.sum* ./
RUN go mod download

COPY . .

ARG VERSION=devel
ARG TARGETOS
ARG TARGETARCH
# CGO_ENABLED=0 is what makes this cross-compile to arm64 for free, which
# matters because a large share of home servers are ARM boards and NAS boxes.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/tabernacle ./cmd/tabernacle


FROM alpine:3

# su-exec drops privileges in the entrypoint; wget (busybox) serves the
# healthcheck. Alpine rather than a distroless base because a homelab operator
# occasionally wants a shell in the container, and it costs a few megabytes.
RUN apk add --no-cache su-exec tzdata ca-certificates \
    && addgroup -g 1000 tabernacle \
    && adduser -D -u 1000 -G tabernacle tabernacle \
    && mkdir -p /data \
    && chown tabernacle:tabernacle /data

LABEL org.opencontainers.image.title="Tabernacle" \
      org.opencontainers.image.description="A self-hosted personal guitar songbook and chord library." \
      org.opencontainers.image.source="https://github.com/jalmena/tabernacle" \
      org.opencontainers.image.licenses="GPL-3.0-only"

COPY --from=build /out/tabernacle /usr/local/bin/tabernacle
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

ENV TABERNACLE_DATA_DIR=/data \
    TABERNACLE_ADDR=:8080 \
    PUID=1000 \
    PGID=1000

VOLUME ["/data"]
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["tabernacle"]
