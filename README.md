# Strapontin

A personal theatre log, shared with a small invite-only circle. Members note the plays they want to see and the ones they have seen, with a rating and a comment, and follow the authors, directors, performers and venues they like. A show is added by pasting the venue page's link: the app reads the page and proposes a record to check.

The interface is in French. It is a single Go binary serving a SolidJS frontend and a SQLite database. The design lives in [ARCHITECTURE.md](ARCHITECTURE.md).

## Run locally

Requirements: Go ≥ 1.26 and Node 24.

```
cd web && npm ci && cd ..
make build
./strapontin serve            # http://localhost:8080, data in ./data
```

On the first start, `serve` logs a setup link: open it to create the admin account. Run `./strapontin` without arguments to see the other commands (`backup`, `export`, `reextract`, `snapshot`). Each flag also reads an environment variable (`PORT`, `DATA_DIR`, `TRUST_PROXY`, `BASE_URL`), listed in ARCHITECTURE §11.

## Develop

```
make dev                      # API on :8080
cd web && npm run dev         # frontend with hot reload on :5173, proxies /api to :8080
make check                    # go vet, go test, tsc, vite build
```

## Container image

Each push to `main` runs `make check` in GitHub Actions, then builds the `Dockerfile` and pushes it to the GitHub container registry:

- `ghcr.io/mpizenberg/strapontin:sha-<commit>`, one tag per commit
- `ghcr.io/mpizenberg/strapontin:latest`

To try an image locally:

```
docker run -p 8080:8080 -v "$PWD/data:/data" ghcr.io/mpizenberg/strapontin:latest
```

## Deploy on Dokku

With the dokku-tui create wizard:

- app `strapontin`, image `ghcr.io/mpizenberg/strapontin:sha-<commit>`, container port `8080`
- no registry login: the package is public
- persistent storage: default host directory, container path `/data`
- env vars `TRUST_PROXY=1` and `BASE_URL=https://<domain>`
- the domain, with Let's Encrypt

Then open the app's logs: the first start prints the setup link that creates the admin account. The plain `dokku` commands are in ARCHITECTURE §11.2.

To deploy a new commit, redeploy with its `sha-<commit>` image. Use the commit tag rather than `latest`: Dokku only redeploys when the image reference changes.
