# GitHub Actions: artifact storage fallback

Two composite actions that keep a release pipeline working when GitHub's
**Actions artifact storage quota** is exhausted (`Error: Failed to
CreateArtifact: Artifact storage quota has been hit`). They wrap the standard
`actions/upload-artifact` / `actions/download-artifact` and transparently divert
artifacts to an **st0r4g9** object store when — and only when — GitHub's own
storage fails.

- [`upload-artifact-fallback`](upload-artifact-fallback/action.yml) — try
  `actions/upload-artifact`; on failure, `PUT` the file(s) into st0r4g9.
- [`download-artifacts-fallback`](download-artifacts-fallback/action.yml) — run
  `actions/download-artifact`, then pull anything that was diverted to st0r4g9.

The fallback path runs **only if the GitHub step actually fails**, so when you
are under quota the behaviour is byte-for-byte the stock actions. If the GitHub
upload fails and no fallback is configured, the step fails loudly (it does not
silently drop the artifact).

## How it works

```
upload:   actions/upload-artifact  --(quota failure)-->  PUT  st0r4g9
download: actions/download-artifact --(then always)---->  GET  st0r4g9 (merge)
```

- **Object layout.** Each file is stored at
  `<fallback-bucket>/<fallback-prefix>/<basename>`. `fallback-prefix` defaults to
  the tag/ref name (`github.ref_name`), so a run for tag `v1.2.3` writes
  `ci-artifacts/v1.2.3/<file>`.
- **Download merge.** The download action lists
  `<bucket>?prefix=<prefix>/`, then downloads every object whose **basename** is
  not already present from GitHub artifacts. Fallback files land in
  `<path>/_fallback/`, so a downstream `find <path> -type f` picks them up
  alongside the GitHub ones. A missing bucket (`404`) means the fallback was
  never needed — nothing to pull.
- **Assumption: basenames are unique across a release.** Because the merge
  deduplicates by basename, two different artifacts must not share a filename.
  Release assets normally already encode platform/arch/kind in the name, so this
  holds; if yours don't, give them distinct names.
- **Same key both sides.** st0r4g9 buckets are namespaced per API key, so the
  upload and download actions must use the *same* `fallback-key`. Wiring both to
  `secrets.ST0R4G9_API_KEY` (as below) guarantees this.

## Prerequisites

1. A reachable st0r4g9 instance. For an ephemeral one, run it behind ngrok from
   the st0r4g9 repo:

   ```bash
   make docker-tunnel   # server + ngrok in containers; inspector at :4040
   # or: make tunnel NGROK_ARGS="--url=my-name.ngrok.app"  (stable URL)
   ```

   The `ngrok-skip-browser-warning: true` header the actions send already
   bypasses the free-tier interstitial. Note that a **random** free-tier URL
   changes every restart — see the config step below.

2. An API key (minted once, reused by CI):

   ```bash
   curl -fsS -XPOST "$ST0R4G9_URL/api-keys" | jq -r .key   # -> st0r_...
   ```

   The bucket is created automatically on first fallback upload; you don't need
   to pre-create it.

## Configure the consumer repository

In the repo that will use these actions, add (Settings → Secrets and variables →
Actions):

| Kind      | Name              | Value                                             |
|-----------|-------------------|---------------------------------------------------|
| Variable  | `ST0R4G9_URL`     | st0r4g9 base URL, e.g. the ngrok `https://…` URL.  |
| Secret    | `ST0R4G9_API_KEY` | The `st0r_…` key minted above.                     |

`ST0R4G9_URL` is a **variable** (not a secret): it isn't sensitive, rotates with
the tunnel, and can be referenced in `if:` conditions. The API key is a
**secret**. A trailing slash on the URL is fine (it is stripped).

> On ngrok's free tier the URL is regenerated on each restart, so update the
> `ST0R4G9_URL` variable whenever you restart the tunnel. A reserved/paid domain
> (`--url=`) avoids this.

## Use in another repo

### Option A — reference remotely (recommended)

Pin to a tag or commit SHA of this repo:

```yaml
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      # ... produce artifacts/app.tar.gz ...
      - name: Upload artifact (with fallback)
        uses: zignd/st0r4g9/.github/actions/upload-artifact-fallback@v1
        with:
          name: app-${{ github.ref_name }}
          path: artifacts/app.tar.gz
          fallback-url: ${{ vars.ST0R4G9_URL }}
          fallback-key: ${{ secrets.ST0R4G9_API_KEY }}
          fallback-bucket: my-project-releases   # optional; default: ci-artifacts

  publish:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Download artifacts (with fallback)
        uses: zignd/st0r4g9/.github/actions/download-artifacts-fallback@v1
        with:
          pattern: app-*
          path: artifacts/release
          fallback-url: ${{ vars.ST0R4G9_URL }}
          fallback-key: ${{ secrets.ST0R4G9_API_KEY }}
          fallback-bucket: my-project-releases
      - name: Use the files
        run: find artifacts/release -type f    # includes artifacts/release/_fallback/*
```

`fallback-bucket` must match on both sides. Composite actions can't read
`secrets`/`vars` directly, which is why they're passed explicitly as inputs.

### Option B — vendor a copy

Copy both action directories into the consumer repo under `.github/actions/` and
reference them by local path (`uses: ./.github/actions/upload-artifact-fallback`).
Use this if you'd rather not depend on this repo's availability at CI time.

## Inputs

### `upload-artifact-fallback`

| Input               | Required | Default             | Notes |
|---------------------|----------|---------------------|-------|
| `name`              | yes      | —                   | GitHub artifact name. |
| `path`              | yes      | —                   | A single file or a directory of files. Globs are **not** expanded by the fallback. |
| `if-no-files-found` | no       | `error`             | Passed through to `actions/upload-artifact`. |
| `fallback-url`      | no       | `""`                | st0r4g9 base URL. Empty ⇒ fallback disabled (and a GitHub failure then fails the step). |
| `fallback-key`      | no       | `""`                | Bearer token. Empty ⇒ fallback disabled. |
| `fallback-bucket`   | no       | `ci-artifacts`      | Bucket name (3–63 chars, `[a-z0-9.-]`). |
| `fallback-prefix`   | no       | `${{ github.ref_name }}` | Key prefix within the bucket. |

### `download-artifacts-fallback`

| Input             | Required | Default                  | Notes |
|-------------------|----------|--------------------------|-------|
| `pattern`         | no       | `*`                      | Passed to `actions/download-artifact`. |
| `path`            | no       | `artifacts/release`      | Download directory; fallback files go to `<path>/_fallback/`. |
| `fallback-url`    | no       | `""`                     | Empty ⇒ GitHub artifacts only. |
| `fallback-key`    | no       | `""`                     | Empty ⇒ GitHub artifacts only. |
| `fallback-bucket` | no       | `ci-artifacts`           | Must match the upload side. |
| `fallback-prefix` | no       | `${{ github.ref_name }}` | Must match the upload side. |

## Requirements & limitations

- The download action runs on a Linux runner (uses `jq`, `mapfile`, and GNU
  `find`); `jq` is preinstalled on GitHub-hosted `ubuntu-*`. The upload action is
  cross-platform (macOS/Windows/Linux) and needs only `bash` + `curl`.
- No multipart upload or checksum verification on download — st0r4g9 streams
  objects to disk and records a SHA-256 ETag, but these actions don't currently
  re-verify it.
- The fallback is best-effort infrastructure for quota exhaustion, not a durable
  release store; treat st0r4g9 contents as disposable once the real artifacts are
  published (e.g. to a GitHub Release).
