# Strapontin

A personal theatre log, shared with a small invite-only circle. Members note the plays they want to see and the ones they have seen, with a rating and a comment, and follow the authors, directors, performers and venues they like. A show is added by pasting the venue page's link: the app reads the page and proposes a record to check.

The interface is in French. It is a single Go binary serving a SolidJS frontend and a SQLite database. The design lives in [ARCHITECTURE.md](ARCHITECTURE.md).

## Run locally

Requirements: Go ≥ 1.26 and Node 24.

```
cd web && npm ci && cd ..
make build
./strapontin admin create     # asks for a name, a login and a password
./strapontin serve            # http://localhost:8080, data in ./data
```

Run `./strapontin` without arguments to see the other commands (`backup`, `export`, `reextract`, `snapshot`). Each flag also reads an environment variable (`PORT`, `DATA_DIR`, `TRUST_PROXY`, `BASE_URL`), listed in ARCHITECTURE §11.

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

One-time setup on the server: create the app, mount `/data` and set the configuration as described in ARCHITECTURE §11.2. The package is private, so Dokku also needs to log in to the registry, with a classic GitHub token that has the `read:packages` scope:

```
dokku registry:login ghcr.io <github-user> <token>
```

Then deploy a commit's image:

```
dokku git:from-image strapontin ghcr.io/mpizenberg/strapontin:sha-<commit>
```

Use the commit tag rather than `latest`: Dokku only redeploys when the image reference changes. Create the first account with `dokku run strapontin admin create`.
