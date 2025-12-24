# MangaHub Backend

Backend services for MangaHub. This repo includes:

- **API server** (HTTP + WebSocket) on `:8080` plus TCP sync on `:7070`.
- **Mirror server** for static demo data on `:9000`.
- **gRPC server** on `:9090`.
- **Scraper job** to populate the SQLite database.

## Prerequisites

### Option A: Docker (recommended)
- Docker + Docker Compose

### Option B: Local Go tooling
- Go **1.24+**
- SQLite development headers (required by `github.com/mattn/go-sqlite3`)
  - macOS: `brew install sqlite`
  - Ubuntu/Debian: `sudo apt-get install -y sqlite3 libsqlite3-dev build-essential`

## Quick Start (Docker Compose)

From the repository root:

```bash
docker compose up --build
```

This starts:

- `mirror` on [http://localhost:9000](http://localhost:9000)
- `api` on [http://localhost:8080](http://localhost:8080)
- `grpc` on `localhost:9090`

Populate the database (one-off job):

```bash
docker compose run --rm scraper
```

Check the API health endpoint:

```bash
curl http://localhost:8080/health
```

Stop everything:

```bash
docker compose down
```

## Run Locally (without Docker)

### 1) Start the mirror server

```bash
go run ./cmd/mirror-server
```

### 2) Start the API server

```bash
go run ./cmd/api-server
```

### 3) (Optional) Start the gRPC server

```bash
go run ./cmd/grpc-server
```

### 4) Populate the database

In another terminal (after the mirror server is running):

```bash
go run ./cmd/scraper
```

The database will be created at `~/.mangahub/data.db` by default.

## Configuration

Environment variables used by the services:

| Variable | Description | Default |
| --- | --- | --- |
| `MANGAHUB_DB_PATH` | SQLite database path | `~/.mangahub/data.db` |
| `MANGAHUB_WEB_ROOT` | Path to UI assets for the API server | `./web` |
| `MANGAHUB_JWT_SECRET` | JWT signing secret | `dev-secret-change-me` |
| `MANGAHUB_JWT_ISSUER` | JWT issuer | `mangahub` |
| `MANGAHUB_JWT_TTL_HOURS` | JWT TTL in hours | `24` |
| `MANGAHUB_GRPC_ADDR` | gRPC listen address | `:9090` |
| `MIRROR_BASE_URL` | Mirror server base URL for scraper | `http://localhost:9000` |
| `MIRROR_DATA_PATH` | Override path to `mirror.json` | `data/mirror.json` |

## Useful Endpoints

- API health: `GET /health`
- API readiness: `GET /ready`
- Mirror titles: `GET http://localhost:9000/titles`
- Web UI: `http://localhost:8080/`