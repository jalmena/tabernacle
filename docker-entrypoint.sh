#!/bin/sh
# Adjust ownership of the data directory and drop privileges before starting.
#
# This exists for one reason: on a NAS or a CasaOS box the data directory is a
# host bind mount created by root, and a container running as a fixed non-root
# user cannot write to it. That is the single most common one-click-install
# failure, so PUID/PGID follow the convention those users already know.
#
# The app itself never runs as root. If the container is already started as a
# non-root user (docker run --user, or `user:` in compose), the chown and the
# privilege drop are both skipped and execution passes straight through.
set -e

DATA_DIR="${TABERNACLE_DATA_DIR:-/data}"

if [ "$(id -u)" = "0" ]; then
    PUID="${PUID:-1000}"
    PGID="${PGID:-1000}"

    if [ "$(id -g tabernacle 2>/dev/null)" != "$PGID" ]; then
        delgroup tabernacle 2>/dev/null || true
        addgroup -g "$PGID" tabernacle 2>/dev/null || true
    fi
    if [ "$(id -u tabernacle 2>/dev/null)" != "$PUID" ]; then
        deluser tabernacle 2>/dev/null || true
        adduser -D -H -u "$PUID" -G tabernacle tabernacle 2>/dev/null || true
    fi

    mkdir -p "$DATA_DIR"
    # Only chown when it is actually wrong: on a large collection stored on a
    # spinning disk or an SMB mount, an unconditional recursive chown at every
    # start is slow enough to be noticed.
    if [ "$(stat -c '%u:%g' "$DATA_DIR")" != "${PUID}:${PGID}" ]; then
        echo "tabernacle: taking ownership of $DATA_DIR as ${PUID}:${PGID}"
        chown -R "${PUID}:${PGID}" "$DATA_DIR" || \
            echo "tabernacle: could not chown $DATA_DIR; continuing anyway" >&2
    fi

    exec su-exec "${PUID}:${PGID}" "$@"
fi

exec "$@"
