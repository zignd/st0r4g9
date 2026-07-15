# st0r4g9

A simplified, S3-flavored object store written in Go. It models the core S3
concepts over a plain RESTful HTTP API:

- **Buckets** — logical containers for objects, created per API key.
- **Objects** — the file bytes plus an object **key** and **metadata**.
- **Keys** — flat string identifiers; a `/` in a key just creates a visual
  "folder" prefix (`folder/hello.txt`).
- **Metadata** — descriptive name/value pairs (content-type, size, timestamps,
  plus your own custom pairs).
- **Annotations** — queryable name/value context you can filter objects by.

Auth is via **API keys**: one open endpoint mints a key; every other endpoint
requires it. Each key owns an **isolated namespace** of buckets — two different
keys can both have a `photos` bucket without collision.

## Architecture

- **HTTP:** [Fiber v3](https://gofiber.io).
- **Metadata store:** SQLite (`modernc.org/sqlite`, pure Go — no CGO).
- **Object bytes:** the local filesystem. Uploads stream straight to disk (never
  buffered whole in memory), so large objects are fine. Each object's blob path,
  size, and SHA-256 (its ETag) are recorded in SQLite.

```
cmd/st0r4g9        entrypoint
internal/config    env-var configuration
internal/store     SQLite: api keys, buckets, objects, metadata, annotations
internal/auth      API-key generation/hashing + Fiber auth middleware
internal/storage   filesystem blob store
internal/api       Fiber routes/handlers
bruno/             Bruno API collection (see below)
```

## Running

Requires Go 1.26+. A `Makefile` wraps the common tasks — run `make help` for the
full list. The most useful:

```bash
make run           # build and run the server
make tunnel        # run the server and expose it publicly via ngrok
make docker-tunnel # run server + ngrok in containers (see below)
make test          # run all tests
make lint          # golangci-lint
make check         # fmt-check + vet + lint + test (what CI should run)
make build         # compile to bin/st0r4g9
make clean         # remove build artifacts and local runtime state
```

`make run` accepts overrides, e.g. `make run ADDR=:8080 DB=/tmp/s.db DATA_DIR=/tmp/blobs`.

`make tunnel` starts the server and opens an [ngrok](https://ngrok.com) tunnel to
it (requires `ngrok` on `PATH` with an auth token configured); Ctrl-C stops both.
Pass extra ngrok flags via `NGROK_ARGS`, e.g.
`make tunnel NGROK_ARGS="--url=my-name.ngrok.app"`. Note: on ngrok's free tier the
first hit returns a browser-warning page — API clients should send the header
`ngrok-skip-browser-warning: true` to bypass it.
Or run directly:

```bash
go run ./cmd/st0r4g9
```

### In containers (server + ngrok)

`make docker-tunnel` runs the whole thing with Docker Compose — our server in one
container and the official `ngrok/ngrok` agent in another — with two bind mounts
from the host:

- **ngrok auth**: your host `~/Library/Application Support/ngrok/ngrok.yml` is
  mounted read-only into the ngrok container, so the tunnel uses your existing
  token. (A small repo overlay, `deploy/ngrok/web.yml`, is merged on top only to
  expose the inspector; your host config is never modified.)
- **data**: the host `./data` directory is mounted at `/data`, so the SQLite DB
  and object blobs live on the host — **drop the containers and the data stays.**

```bash
make docker-tunnel     # build images, start server + ngrok (Ctrl-C stops both)
make docker-down       # stop and remove the containers; host data is preserved
```

The server is published on `localhost:9000` and ngrok's inspector on
`http://localhost:4040` (shows the public URL). Overridable variables:
`NGROK_CONFIG` (host ngrok.yml path), `DOCKER_DATA_DIR` (host data dir),
`SERVER_PORT`, `NGROK_INSPECTOR_PORT`. Extra ngrok flags for the non-container
`make tunnel` go via `NGROK_ARGS`.

Configuration (all optional):

| Env var            | Default    | Meaning                        |
|--------------------|------------|--------------------------------|
| `ST0R4G9_ADDR`       | `:9000`    | HTTP listen address                       |
| `ST0R4G9_DB`         | `store.db` | SQLite database file path                 |
| `ST0R4G9_DATA_DIR`   | `data`     | root dir for object blobs                 |
| `ST0R4G9_LOG_LEVEL`  | `info`     | log verbosity: `debug`/`info`/`warn`/`error` |
| `ST0R4G9_LOG_FORMAT` | `text`     | log output format: `text` or `json`       |

## API

Auth header on every request except `POST /api-keys`:
`Authorization: Bearer st0r_...`

| Method   | Path                | Purpose                                                    |
|----------|---------------------|------------------------------------------------------------|
| `POST`   | `/api-keys`         | Mint an API key (open). Returns the raw key **once**.      |
| `GET`    | `/`                 | List the caller's buckets.                                 |
| `PUT`    | `/{bucket}`         | Create a bucket (409 if the caller already has it).        |
| `HEAD`   | `/{bucket}`         | Bucket exists?                                             |
| `DELETE` | `/{bucket}`         | Delete a bucket (409 if non-empty; `?force=true` overrides).|
| `GET`    | `/{bucket}`         | List objects. `?prefix=`, `?delimiter=/`, `?annotation.<n>=<v>`. |
| `PUT`    | `/{bucket}/{key}`   | Upload/overwrite an object (body = bytes).                 |
| `GET`    | `/{bucket}/{key}`   | Download an object.                                        |
| `HEAD`   | `/{bucket}/{key}`   | Object metadata only.                                      |
| `DELETE` | `/{bucket}/{key}`   | Delete an object.                                          |

Custom headers on `PUT` object:

- `X-Str-Meta-<name>: <value>` — descriptive metadata.
- `X-Str-Annotation-<name>: <value>` — queryable annotation.

Both are echoed back as response headers on `GET`/`HEAD`. Errors are JSON:
`{"error":{"code":"NoSuchBucket","message":"..."}}`.

## Quickstart (curl)

```bash
# 1. Mint a key
KEY=$(curl -s -XPOST localhost:9000/api-keys | jq -r .key)

# 2. Create a bucket
curl -XPUT -H "Authorization: Bearer $KEY" localhost:9000/photos

# 3. Upload an object with metadata + an annotation
curl -XPUT -H "Authorization: Bearer $KEY" \
     -H "Content-Type: text/plain" \
     -H "X-Str-Meta-Author: igor" \
     -H "X-Str-Annotation-project: demo" \
     --data-binary "hello world" \
     localhost:9000/photos/folder/hello.txt

# 4. Download it
curl -H "Authorization: Bearer $KEY" localhost:9000/photos/folder/hello.txt

# 5. List with a prefix / fold folders
curl -H "Authorization: Bearer $KEY" "localhost:9000/photos?prefix=folder/&delimiter=/"

# 6. Query by annotation
curl -H "Authorization: Bearer $KEY" "localhost:9000/photos?annotation.project=demo"
```

## Logging

Every request produces one structured [`log/slog`](https://pkg.go.dev/log/slog)
access-log line — method, path, status, latency, response bytes, and client IP —
logged at a level chosen by status (`5xx` → error, `4xx` → warn, else info).
Server-side failures also carry an `error` field explaining what went wrong.
Set `ST0R4G9_LOG_FORMAT=json` for machine-readable output and
`ST0R4G9_LOG_LEVEL=debug` to also see an "incoming request" line per request.

```
time=... level=WARN msg=request method=GET path=/photos/missing.txt status=404 latency_ms=0.11 bytes=70 ip=127.0.0.1
time=... level=ERROR msg=request method=PUT path=/photos/x.txt status=500 ... error="mkdir data/1: permission denied"
```

## Bruno collection

`bruno/` is a [Bruno](https://usebruno.com) collection covering every endpoint,
ordered so a fresh run flows top to bottom. Open it in the Bruno app (or run it
with the CLI), pick the **Local** environment, and send **Create API Key** first —
its post-response script stores the returned key in the `apiKey` env var, and the
rest of the requests inherit `Authorization: Bearer {{apiKey}}` from the
collection automatically.

```bash
# optional: run the whole collection headless
cd bruno && bru run --env Local
```

## Tests

```bash
make test        # or: go test ./...
```

Covers the store (CRUD + per-key isolation), auth (key gen/hash + middleware),
the blob store (write/read/delete, hashing, empty objects), and full HTTP
integration (happy path, error codes, cross-namespace isolation).

## CI artifact fallback (GitHub Actions)

`.github/actions/` ships two reusable composite actions that use st0r4g9 as a
drop-in fallback for GitHub's Actions artifact storage, so a release pipeline
keeps working when the artifact **storage quota** is exhausted. They wrap
`actions/upload-artifact` / `actions/download-artifact` and only divert to
st0r4g9 when GitHub's own storage fails. See
[`.github/actions/README.md`](.github/actions/README.md) for setup and usage in
another repo.

## Scope

Deliberately omitted for simplicity: multipart uploads, versioning, presigned
URLs, ACLs beyond key ownership, region routing, and TLS.
