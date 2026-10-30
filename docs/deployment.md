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

There is a store, so Tabernacle installs like any other app rather than being
pasted in by hand. App Store → **Add Source**, and paste

```
https://raw.githubusercontent.com/jalmena/jalmena-appstore/gh-pages/appstore.zip
```

Tabernacle then appears in the store and updates itself when a new version is
published. On ZimaOS, or any client that speaks version 2 of the protocol, the
same store is also served as `index.json` from the same branch. The store lives
in [its own repository](https://github.com/jalmena/jalmena-appstore), shared with
the author's other apps, which carries the compose file CasaOS installs and the
metadata that draws its card.

Custom Install still works: App Store → **Custom Install** → *Import* → paste
`compose.yaml`. The `x-casaos` blocks in that file give CasaOS the title, icon,
description and port mapping, so it renders a real app card rather than a bare
container, and plain Docker Compose ignores them — which is why one file serves
both. Adjust the volume to the platform's convention before installing:

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

Reading a Tabernacle needs no account, so anything that can reach it can read the
collection. That is fine on a trusted network and unacceptable on the open
internet, so put it behind a VPN — WireGuard or Tailscale — or behind an
authenticating proxy. Changing anything needs an account once the instance has
been claimed; see [accounts](accounts.md).

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

Two checks, two questions. That allowlist answers "which names does this server
answer to", and it is deliberately generous, because a home server is reached by
half a dozen names nobody can list in advance. A separate check answers "which
pages may submit a change" — see [accounts](accounts.md). The practical
consequence for a deployment is that a domain on a non-standard port belongs in
`TABERNACLE_BASE_URL` **with** its port, or writes from it are refused.

### Sharing the library read-only

```yaml
environment:
  TABERNACLE_READ_ONLY: "true"
```

Every change is refused and the editing controls disappear. Reading, searching,
transposing, printing and exporting all keep working. Useful for a second
instance pointed at the same directory, or for handing the address to someone at
a rehearsal.

## Accounts

A fresh instance has none, and says so on every page: until it is claimed, anybody
who can reach it can change the collection. Close that either from the browser —
**Acceder**, then **Ajustes** — or before the first start:

```yaml
environment:
  TABERNACLE_ADMIN_USER: "you"
  TABERNACLE_ADMIN_PASSWORD: "a long password"
```

Both or neither; one alone is a configuration error. They are read at startup
only, so remove them afterwards. Setting them again resets that account's password
and restores its administrator role, which is the way back in after a forgotten
one.

Full detail in [accounts](accounts.md), including the other two recovery routes.

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

Two things need backing up, and they are both in the data directory:

- `songs/` — the collection. This is the whole point.
- `accounts.json` — the accounts and the key that signs sessions. It is the one
  file here that cannot be regenerated.

`index.db` needs no backup: it is derived from the songs and rebuilds itself on
the next start, with the same addresses as before. Copying it anyway is harmless.

```sh
tar czf tabernacle-$(date +%F).tar.gz -C /path/to/data songs accounts.json
```

Restoring is putting them back. Nothing else has to happen: the next start
notices the songs and indexes them.

## Upgrading

```sh
docker compose pull && docker compose up -d
```

Songs are plain files and the index is disposable, so there is nothing to
migrate. A version that changed the index layout rebuilds it on first start.
