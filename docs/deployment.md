# Deploying Tabernacle

Tabernacle is one static binary that listens on plain HTTP. TLS, access control
and anything else belongs in front of it. This document covers the shapes that
actually come up in a homelab.

## What Tabernacle expects

- A directory it owns, holding `songs/`, `trash/` and `index.db`.
- Somewhere to listen. Default `:8080`.
- Nothing else. No database server, no cache, no internet.

## Docker Compose

```sh
docker compose up -d
```

`compose.yaml` in the repository root is ready to use. The two things worth
changing are the volume and `PUID`/`PGID`.

```yaml
volumes:
  - /srv/tabernacle:/data
environment:
  PUID: "1000"    # id -u
  PGID: "1000"    # id -g
```

If the container cannot write to the volume, nothing else will work. `PUID` and
`PGID` are the usual cause and the usual fix.

## CasaOS and ZimaOS

App Store → **Custom Install** → *Import* → paste `compose.yaml`.

The `x-casaos` blocks in that file give CasaOS the title, icon, description and
port mapping, so it renders a real app card rather than a bare container. Plain
Docker Compose ignores those blocks, which is why one file serves both.

Adjust the volume to the platform's convention before installing:

```yaml
volumes:
  - /DATA/AppData/tabernacle/data:/data
```

## Unraid, Synology, Portainer

Any of them can take `compose.yaml` directly, or create a container from
`ghcr.io/jalmena/tabernacle:latest` with `/data` mapped to a share. Images are
published for `linux/amd64` and `linux/arm64`, so an ARM NAS or a Raspberry Pi
works the same as an x86 box.

## Behind a domain

Put your own reverse proxy in front and terminate TLS there; Tabernacle never
sees a certificate. It does need telling what it is called, or it will refuse
requests for that name:

```yaml
environment:
  TABERNACLE_BASE_URL: "https://songs.example.com"
  TABERNACLE_ALLOWED_HOSTS: "songs.example.com"
```

### Why that second variable matters

Tabernacle has no accounts. Anything that can reach it can change the
collection. That is fine on a trusted network and unacceptable on the open
internet, so put it behind a VPN — WireGuard or Tailscale — or behind an
authenticating proxy.

There is one attack that a firewall does not stop, and it is the reason for the
host allowlist. A website you visit can point its own hostname at your NAS's
private address, and the script on that page then reaches Tabernacle from your
browser, inside your network. Checking the `Origin` header does not help: the
attacker's origin is exactly what the browser sends.

Out of the box Tabernacle accepts requests addressed to private addresses,
`localhost` and names like `nas.local`, and refuses public hostnames. That closes
the hole with no configuration, which is why a bare `docker run` is safe on a
LAN. The moment you put a real domain in front of it, that name has to be
allowlisted explicitly — hence `TABERNACLE_ALLOWED_HOSTS`.

### Sharing the library read-only

```yaml
environment:
  TABERNACLE_READ_ONLY: "true"
```

Every change is refused and the editing controls disappear. Reading, searching,
transposing, printing and exporting all keep working. Useful for a second
instance pointed at the same directory, or for handing the address to someone at
a rehearsal.

## Running without Docker

```sh
go build ./cmd/tabernacle
TABERNACLE_DATA_DIR=/srv/tabernacle ./tabernacle
```

A systemd unit, if you want one:

```ini
[Unit]
Description=Tabernacle
After=network-online.target

[Service]
ExecStart=/usr/local/bin/tabernacle
Environment=TABERNACLE_DATA_DIR=/srv/tabernacle
Environment=TABERNACLE_ADDR=127.0.0.1:8080
User=tabernacle
Restart=on-failure
# Nothing outside the data directory is ever written.
ProtectSystem=strict
ReadWritePaths=/srv/tabernacle
PrivateTmp=yes
NoNewPrivileges=yes

[Install]
WantedBy=multi-user.target
```

## Health

`GET /healthz` returns JSON:

```json
{"status":"ok","songs":128,"artists":41,"broken":0}
```

It reports healthy even when the collection is empty or some files do not parse.
Those are things for you to see in the interface, not reasons for a container
runtime to restart a working application. It reports `503` only when the index is
unreachable, which is the one condition the application cannot serve around.

## Backup

Copy the songs directory:

```sh
tar czf tabernacle-$(date +%F).tar.gz /srv/tabernacle/songs
```

Or use **Importar / Exportar** in the interface, which downloads the same thing
as a zip.

`index.db` needs no backup. Delete it and Tabernacle rebuilds it on the next
start, with the same URLs as before — slugs are derived from file paths precisely
so that a rebuild cannot break your bookmarks. What is lost is the small
conveniences: which songs you opened recently, and any key you asked it to
remember. Favourites survive, because they are a tag in the song file.

## Upgrading

```sh
docker compose pull && docker compose up -d
```

Songs are plain files and the index is disposable, so there is nothing to
migrate. A version that changed the index layout rebuilds it on first start.
