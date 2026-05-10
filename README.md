# Sila

Self-hosted blocklist aggregator — fetches multiple third-party adblock/blocklists, deduplicates them, applies a user-managed whitelist, and serves the merged output as a single blocklist. Includes a web-based admin UI.

## Quickstart

```bash
# Build
go build -o sila ./cmd/server/

# Run (creates data/sila.db, seeds two default lists, starts fetching)
./sila

# Open http://localhost:8080
```

## Docker

Pull the image from GHCR:

```bash
docker pull ghcr.io/klppl/sila:latest
```

Run it:

```bash
docker run -d \
  --name sila \
  -p 8080:8080 \
  -v /path/on/host:/data \
  -e SILA_ADMIN_PASSWORD=your-secure-password \
  ghcr.io/klppl/sila:latest
```

Replace `/path/on/host` with a persistent directory on your VPS and set a strong `SILA_ADMIN_PASSWORD`. All env vars listed below in the Configuration table can be passed with `-e`.

## Configuration

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP listen port |
| `SILA_DB_PATH` | `data/sila.db` | SQLite database path |
| `SILA_DATA_DIR` | `data` | Data directory |
| `SILA_REFRESH_INTERVAL` | `6h` | Auto-refresh interval (Go duration format) |
| `SILA_ADMIN_PASSWORD` | `hunter2` | Password for the admin UI (username: `admin`) |

All config is via environment variables. No config file needed.

## Endpoints

### Public

| Method | Path | Description |
|---|---|---|
| `GET` | `/` | Public landing page — stats, source lists, usage guide |
| `GET` | `/blocklist.txt` | Merged blocklist in Adblock Plus syntax, filtered by whitelist |

### Admin (password-protected via `/web/login`, session-based)

| Method | Path | Description |
|---|---|---|
| `GET` | `/web` | Dashboard — domain count, list count, last refresh, manual refresh |
| `GET` | `/web/lists` | Manage source list subscriptions (add, toggle, delete) |
| `GET` | `/web/whitelist` | Manage whitelist entries (exact + wildcard `*.example.com`) |
| `GET` | `/web/search` | Search domains to see which source lists they come from |

## Input formats supported

- **Hosts file**: `0.0.0.0 domain.com`, `127.0.0.1 domain.com`
- **Adblock Plus**: `||domain.com^`

## Default lists

On first run, two lists are seeded:

- [OISD Small](https://small.oisd.nl)
- [Frellwit Swedish](https://raw.githubusercontent.com/lassekongo83/Frellwits-filter-lists/refs/heads/master/Frellwits-Swedish-Filter.txt)

Add or remove lists from the admin UI at `/web/lists`.

## Deploying behind a reverse proxy

Sila listens on `$PORT` (default 8080). Point your reverse proxy at it:

```caddy
blocklist.example.com {
    reverse_proxy localhost:8080
}
```

## Project layout

```
cmd/server/main.go        Entry point
internal/store/           SQLite schema, migrations, CRUD
internal/parser/          Hosts + ABP syntax parser
internal/fetcher/         HTTP fetcher with ETag/Last-Modified caching
internal/scheduler/       Background refresh goroutine
internal/server/          Chi router, handlers, embedded HTML templates
Dockerfile                Multi-stage Alpine build
```
