# Strapontin: architecture

Strapontin is a personal theatre log. You record plays you want to see and plays you have seen, with comments, and you follow your favourite authors, directors, actors and venues. It is shared with a small, invite-only circle of friends and family.

Guiding principle: **keep it simple, minimise dependencies, make it last.**

Background research on data sources:
- `reports/Sources de données théâtre en France.md`
- `reports/Sources de données théâtre Toulouse.md`

## 1. Goals and non-goals

**Goals**

- Open a URL on a phone and use the app, with nothing to install and no VPN.
- Record a show in under a minute: paste a venue page URL or search by title, check the proposed record, save it.
- Keep a durable memory. Venue sites purge their pages, so the app keeps its own copy of everything a member cared about.
- Serve a small, intimate circle (tens of members, never hundreds), with visibility on how it grows.

**Non-goals**

- No crawling, no daily sync, no bulk import from any source. Every external request is triggered by a member action.
- No redistribution of third-party data. Raw page copies are never shown to members.
- No open registration, no public pages, no third-party analytics.
- No offline mode in v1.

## 2. Overview

```
 phone / laptop browser
        │  HTTPS
        ▼
  Dokku nginx (TLS via Let's Encrypt plugin, domain)          ── on the VPS
        │  HTTP, $PORT
        ▼
┌──────────────── container: single Go binary (distroless) ──────────────────┐
│  middleware: cross-origin protection → session → member → role             │
│  ├── static frontend (SolidJS build, embedded)                             │
│  ├── JSON API  /api/...                                                    │
│  ├── extract/  page → structured draft                                     │
│  └── sources/  OpenAgenda, Toulouse tourism, Wikidata (on demand)          │
└───────────────┬────────────────────────────────────────────────────────────┘
                │ Dokku persistent storage mounted at /data
                ▼
  SQLite file (WAL) + daily VACUUM INTO backups ──► off-site copy from the host
```

Why there is a backend: browsers cannot fetch venue pages directly because of CORS. Something must fetch, snapshot and parse them. Once that server exists, it also owns the database and serves the frontend.

## 3. Stack and dependencies

| Layer | Choice | Dependencies |
|---|---|---|
| Backend | Go ≥ 1.26, standard library `net/http`, `database/sql`, `embed`, `testing` | `modernc.org/sqlite` (pure Go, FTS5), `golang.org/x/net/html`, `golang.org/x/crypto` (argon2id) |
| Database | SQLite, WAL mode, a single file on a persistent volume | — |
| Frontend | SolidJS 2, TypeScript, plain CSS | runtime: `solid-js`, `@solidjs/web`; dev: `vite`, `@solidjs/vite-plugin`, `typescript` |
| Distribution | container image: multi-stage `Dockerfile` with a distroless static base | — |
| Hosting | local first; later Dokku on your VPS, whose nginx handles TLS and the domain | — |

That is 3 Go modules and 5 direct npm packages, all from the Go project or the Solid, Vite and TypeScript projects. The frontend toolchain installs 43 npm packages in total.

**Deliberately absent**

- No ORM and no query generator. SQL is written by hand.
- No migration tool. Numbered `.sql` files are embedded in the binary and tracked with `PRAGMA user_version`.
- No router library: History API routing is hand-written in about 40 lines.
- No CSS framework or preprocessor.
- No TLS or certificate code in the app: Dokku's nginx handles both.
- No container orchestration beyond Dokku, and no external authentication provider.

**Why Go**

- The standard library covers almost everything.
- It builds a single static binary (`CGO_ENABLED=0`), so the container image is that binary plus CA certificates, about 20 MB.
- The Go 1 compatibility promise means the code should still build in ten years.

**Alternatives considered**

- **Bun/TypeScript on both sides.** It gives one language, but extracting text from HTML is awkward there, Solid's JSX needs a plugin, and the runtime is less stable over time.
- **Rust.** It means far more transitive dependencies.
- **PostgreSQL.** It is pointless at this scale.
- **Tailscale or proxy authentication.** It is too much friction for family members.
- **One database per member.** It gives up the shared catalogue.

## 4. Data model

The data comes in three layers: a **shared catalogue** that any member can enrich, a **personal layer** that belongs to each member, and **accounts**.

Identifiers are `INTEGER PRIMARY KEY`, since there is a single database and no sync. Timestamps are ISO 8601 UTC text.

### 4.1 Shared catalogue

```sql
CREATE TABLE person (
  id          INTEGER PRIMARY KEY,
  name        TEXT NOT NULL,          -- as displayed: "Julie Deliquet"
  name_key    TEXT NOT NULL,          -- lowercased, accents folded: used for matching
  created_by  INTEGER REFERENCES member(id) ON DELETE SET NULL,
  created_at  TEXT NOT NULL
);

CREATE TABLE work (                   -- the text: "Le Misanthrope"
  id INTEGER PRIMARY KEY, title TEXT NOT NULL,
  created_by INTEGER REFERENCES member(id) ON DELETE SET NULL, created_at TEXT NOT NULL
);

CREATE TABLE venue (
  id INTEGER PRIMARY KEY, name TEXT NOT NULL, city TEXT, address TEXT,
  created_by INTEGER REFERENCES member(id) ON DELETE SET NULL, created_at TEXT NOT NULL
);

CREATE TABLE production (             -- a specific staging; work_id is NULL for devised pieces
  id INTEGER PRIMARY KEY, title TEXT NOT NULL,
  work_id INTEGER REFERENCES work(id), company TEXT, description TEXT,
  created_by INTEGER REFERENCES member(id) ON DELETE SET NULL, created_at TEXT NOT NULL
);

CREATE TABLE run (                    -- a production at one venue between two dates
  id            INTEGER PRIMARY KEY,
  production_id INTEGER NOT NULL REFERENCES production(id) ON DELETE CASCADE,
  venue_id      INTEGER NOT NULL REFERENCES venue(id),
  starts_on     TEXT, ends_on TEXT     -- dates, when known
);

CREATE TABLE credit (
  id            INTEGER PRIMARY KEY,
  production_id INTEGER REFERENCES production(id) ON DELETE CASCADE,
  work_id       INTEGER REFERENCES work(id) ON DELETE CASCADE,
  person_id     INTEGER NOT NULL REFERENCES person(id),
  role          TEXT NOT NULL,        -- see the role vocabulary below
  character     TEXT,                 -- for performers
  position      INTEGER NOT NULL DEFAULT 0,
  CHECK ((production_id IS NULL) <> (work_id IS NULL))
);

CREATE TABLE external_id (            -- links to stable outside identifiers
  entity     TEXT NOT NULL CHECK (entity IN ('person','work','venue','production')),
  entity_id  INTEGER NOT NULL,
  scheme     TEXT NOT NULL,           -- wikidata, bnf, idref, isni, ads, deps, siret, openagenda, url
  value      TEXT NOT NULL,
  PRIMARY KEY (scheme, value)
);

CREATE TABLE snapshot (               -- raw page copy, used internally only
  id                INTEGER PRIMARY KEY,
  url               TEXT NOT NULL,
  fetched_at        TEXT NOT NULL,
  fetched_by        INTEGER REFERENCES member(id) ON DELETE SET NULL,
  http_status       INTEGER NOT NULL,
  content_type      TEXT,
  body              BLOB NOT NULL,    -- gzip
  sha256            TEXT NOT NULL,
  extractor_version INTEGER NOT NULL,
  extracted         TEXT,             -- JSON draft produced by extract/
  production_id     INTEGER REFERENCES production(id) ON DELETE SET NULL
);

CREATE TABLE image (                  -- posters disappear as well
  id INTEGER PRIMARY KEY, production_id INTEGER NOT NULL REFERENCES production(id) ON DELETE CASCADE,
  source_url TEXT NOT NULL, content_type TEXT NOT NULL, bytes BLOB NOT NULL, fetched_at TEXT NOT NULL
);

CREATE TABLE revision (               -- light edit history for the shared catalogue
  id INTEGER PRIMARY KEY, entity TEXT NOT NULL, entity_id INTEGER NOT NULL,
  member_id INTEGER REFERENCES member(id) ON DELETE SET NULL, at TEXT NOT NULL,
  before TEXT NOT NULL                -- JSON of the row and its credits before the edit
);
```

**Role vocabulary.** The roles are a closed list in Go, stored as English keys and displayed with French labels:

| Key | French label |
|---|---|
| `author` | auteur |
| `adaptation` | adaptation |
| `translation` | traduction |
| `director` | mise en scène |
| `assistant_director` | assistant(e) à la mise en scène |
| `performer` | interprète |
| `dramaturgy` | dramaturgie |
| `set_design` | scénographie |
| `lighting` | lumière |
| `sound` | son |
| `music` | musique |
| `costumes` | costumes |
| `choreography` | chorégraphie |
| `video` | vidéo |

### 4.2 Personal layer

```sql
CREATE TABLE outing (                 -- want → seen
  id            INTEGER PRIMARY KEY,
  member_id     INTEGER NOT NULL REFERENCES member(id) ON DELETE CASCADE,
  production_id INTEGER NOT NULL REFERENCES production(id),
  run_id        INTEGER REFERENCES run(id) ON DELETE SET NULL,   -- where it was seen
  status        TEXT NOT NULL CHECK (status IN ('want','seen')),
  date          TEXT,                 -- performance date, when known
  rating        INTEGER CHECK (rating BETWEEN 1 AND 5),
  comment       TEXT,
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);

CREATE TABLE favorite (
  member_id INTEGER NOT NULL REFERENCES member(id) ON DELETE CASCADE,
  entity TEXT NOT NULL CHECK (entity IN ('person','work','venue')), entity_id INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (member_id, entity, entity_id)
);

CREATE TABLE note (
  member_id INTEGER NOT NULL REFERENCES member(id) ON DELETE CASCADE,
  entity TEXT NOT NULL, entity_id INTEGER NOT NULL, body TEXT NOT NULL, updated_at TEXT NOT NULL,
  PRIMARY KEY (member_id, entity, entity_id)
);
```

Rules for this layer:
- A wish becomes a viewing by changing its status. Seeing a show twice means two outings.
- The personal layer is **private to its member**. The admin sees counts, never comments, ratings or notes.
- Favourites trigger no alerts, since nothing is monitored. They filter searches and highlight your people inside casts.

### 4.3 Accounts

```sql
CREATE TABLE member (
  id            INTEGER PRIMARY KEY,
  name          TEXT NOT NULL,
  login         TEXT NOT NULL UNIQUE,
  password_hash TEXT,                 -- argon2id; NULL until set
  role          TEXT NOT NULL CHECK (role IN ('admin','member')),
  invited_by    INTEGER REFERENCES member(id) ON DELETE SET NULL,
  created_at    TEXT NOT NULL,
  last_seen_at  TEXT,                 -- updated at most once an hour
  disabled_at   TEXT
);

CREATE TABLE session (
  token_hash TEXT PRIMARY KEY,        -- sha256 of a 32-byte random token
  member_id  INTEGER NOT NULL REFERENCES member(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL, expires_at TEXT NOT NULL, user_agent TEXT
);

CREATE TABLE invite (                 -- also used for one-time password reset links
  token_hash TEXT PRIMARY KEY,
  kind       TEXT NOT NULL CHECK (kind IN ('signup','reset')),
  created_by INTEGER NOT NULL REFERENCES member(id) ON DELETE CASCADE,
  for_member INTEGER REFERENCES member(id) ON DELETE CASCADE,   -- reset only
  created_at TEXT NOT NULL, expires_at TEXT NOT NULL,
  used_by    INTEGER REFERENCES member(id) ON DELETE SET NULL, used_at TEXT
);

CREATE TABLE event (                  -- feeds the admin page
  id INTEGER PRIMARY KEY, member_id INTEGER REFERENCES member(id) ON DELETE SET NULL,
  kind TEXT NOT NULL,                 -- signup, login, login_failed, invite_created, invite_used,
                                      -- import, fetch, source_search
  at TEXT NOT NULL
);

CREATE TABLE setting (key TEXT PRIMARY KEY, value TEXT NOT NULL);
-- max_members (default 30), invite_quota (default 1 invite per member; admins unlimited), ntfy_topic
```

Full-text search uses an FTS5 table over production titles, person names, work titles and venue names. A member's own comments are indexed separately and searched only for that member.

## 5. Import pipeline (`internal/extract`)

Every step is triggered by a member.

1. **Fetch.**
   - Only `http` and `https` URLs are accepted, and private or loopback addresses are refused (SSRF guard).
   - Requests time out after 10 s, bodies are capped at 5 MB, and at most 5 redirects are followed.
   - The User-Agent is `strapontin (personal theatre log)`.
   - If a snapshot of the same URL is less than 24 h old, it is reused. Links are never followed.
2. **Snapshot.** The body is stored gzipped with its hash, whether or not extraction succeeds.
3. **Structured data**, tried in this order:
   1. JSON-LD (`Event`, `TheaterEvent`, `CreativeWork`)
   2. microdata
   3. OpenGraph
   4. `<title>` and `<meta name="description">`
4. **Credits from text.**
   - Labelled text is found with a dictionary of French labels: "Texte", "D'après", "Mise en scène", "Avec", "Interprétation", "Jeu", "Traduction", "Scénographie", "Lumière(s)", "Son", "Costumes", "Musique", "Chorégraphie", "Dramaturgie", "Collaboration artistique", and so on.
   - Names are split on commas, "et", "&" and "/".
   - Rules specific to one site live in a small map keyed by host, added only when a venue needs them (for example the Théâtre de la Cité and Garonne, whose credits run together).
5. **Match.** Proposed names are compared with existing `person.name_key` values, and existing productions with the `external_id` `url` value. Wikidata search is offered when a member wants to link a person.
6. **Draft.** The API returns a draft. The member corrects it in the UI and saves. Saving creates or updates catalogue rows, writes a `revision` and links the snapshot.
7. **Image.** The `og:image` or JSON-LD `image` is downloaded once and stored in `image`.

`extractor_version` is bumped whenever the parser changes. A CLI command re-runs extraction on stored snapshots without touching the sites again.

**Tests.** Stored snapshots double as fixtures: `internal/extract/testdata/*.html.gz`, with golden `*.json` files and standard `go test`.

**Untrusted content.** Everything extracted is rendered as text only, never as HTML. Raw snapshots are never served to members. The admin can inspect them through the CLI.

## 6. External sources (`internal/sources`)

Each source is one file returning a common `Candidate{Title, Venue, Dates, URL, Source}` type, and is called only from a member's search.

| Source | Use | Notes |
|---|---|---|
| OpenAgenda | title search, Toulouse area | Agendas 50522407 (Toulouse Métropole) and 42448083 (city of Toulouse). Uses the keyless endpoint `openagenda.com/api/agendas/{uid}/events?search=…`, with no API key. That endpoint is internal and undocumented, so all of it stays in `openagenda.go`. If it breaks, the search shows "source unavailable" and URL import still works. |
| Toulouse tourism office | title search for venues absent from OpenAgenda | `wp-json/wp/v2/agenda?search=`. Finished events disappear. |
| Wikidata | linking people and works | `wbsearchentities` |
| BnF, IdRef | later | identifiers for authors |
| Les Archives du spectacle | link only | `external_id` scheme `ads` |

## 7. Members, invitations and the admin page

**Registration is by invite only.**
1. An admin creates a single-use signup link that expires after 7 days. Each member can also create one invite (`invite_quota` = 1); admins have no limit.
2. Opening it asks for a name and a password, then signs the person in.
3. Redemption is refused once `max_members` is reached, and the admin is notified.

The first admin is created from the CLI: `strapontin admin create`.

**Sessions**
- The cookie is HttpOnly, Secure, `SameSite=Lax` and `Path=/`.
- It lasts 365 days and is renewed with use.
- The database stores only the token's hash.

On iPhone, an app added to the home screen has its own cookies, so members sign in once more after adding it.

**Password reset.** There is no email system. The admin generates a one-time reset link (`invite.kind = 'reset'`) and sends it through any messaging app.

**Admin controls.** The admin can suspend a member, which sets `disabled_at` and deletes their sessions, change the settings and generate invites.

**Admin page** (`/admin`, role `admin`), built only from SQL over the tables above:

- **Members:** name, inviter, sign-up date, last seen, number of outings and imports.
- **Growth:** sign-ups per month, and the invite tree with its depth. Depth ≥ 3 is the signal that the circle is spreading.
- **Activity:** members active in the last 7 and 30 days, and imports and fetches per day.
- **Health:** database size, snapshot and image volume, date of the last backup.
- **Notifications:** an optional ntfy push when someone joins (a plain HTTP POST with no dependency). The message carries no name, because ntfy.sh is a third party.

**Members' rights.** A member can export their data (JSON and CSV) and delete their account. Deletion removes the personal layer, while catalogue contributions stay with `created_by` set to NULL.

## 8. HTTP API

All endpoints are JSON. Every endpoint except the public ones requires a session.

```
GET    /healthz                       public: 200 once the database is open
POST   /api/login                     public, rate-limited
POST   /api/logout
GET    /api/invites/{token}           public: check an invite
POST   /api/invites/{token}/redeem    public: create the account or reset the password
GET    /api/me

POST   /api/import          {url}  → draft
POST   /api/import/save     draft  → production
GET    /api/sources/search  ?q=&source=openagenda|tourisme
GET    /api/wikidata/search ?q=

GET    /api/search          ?q=     local FTS
GET|POST|PATCH  /api/productions[/{id}]
GET|POST|PATCH  /api/people[/{id}]       people include their credits across productions
GET|POST|PATCH  /api/works[/{id}]
GET|POST|PATCH  /api/venues[/{id}]
GET|POST|PATCH|DELETE /api/outings[/{id}] ?status=     own outings only
PUT|DELETE      /api/favorites/{entity}/{id}
PUT|DELETE      /api/notes/{entity}/{id}
GET    /api/export.json | /api/export.csv               own data only

GET    /api/admin/overview | /api/admin/members | /api/admin/events
POST   /api/admin/invites | /api/admin/members/{id}/disable | /api/admin/members/{id}/reset
PATCH  /api/admin/settings
```

Any path that is not under `/api/` or a static asset serves `index.html`.

## 9. Frontend (`web/`)

```
web/
  index.html  manifest.webmanifest  vite.config.ts  package.json  tsconfig.json
  src/
    main.tsx  router.ts  api.ts  types.ts
    views/       Journal, Wishlist, Add, Production, Person, Work, Venue, Search,
                 Login, Invite, Admin
    components/  CreditEditor, PersonPicker, OutingForm, Rating, ...
  styles/
    tokens.css  base.css  components.css
```

- **Solid 2 toolchain.** Verified on 2026-11-22 with a test build and `tsc --noEmit`:

  | Package | Version | Role |
  |---|---|---|
  | `solid-js` | 2.0.3 | reactivity |
  | `@solidjs/web` | 2.0.3 | DOM rendering and JSX types |
  | `@solidjs/vite-plugin` | 3.0.1 | Vite integration |
  | `vite` | 8.3.8 | build tool |
  | `typescript` | 7.0.2 | type checking |

  - Use **`@solidjs/vite-plugin`**, not `vite-plugin-solid`. The latter (2.11.x) only supports Solid 1.
  - The plugin compiles JSX with the native `@solidjs/compiler`, which falls back to WebAssembly. Babel is an optional peer dependency and is not installed.
  - `render` is imported from `@solidjs/web`, and `tsconfig.json` sets `"jsxImportSource": "@solidjs/web"`.
  - The local npm configuration has `min-release-age` set to 7 days. Solid 2.0.0 was published on 2026-11-17, so the Solid packages (`solid-js`, `@solidjs/web`, `@solidjs/vite-plugin`) are installed as an explicit exception: `npm install --min-release-age=0 <those packages>`. That only changes how versions are picked; the lockfile pins the result. Other packages follow the normal rule.
- **Views.**
  - The journal lists seen outings by date.
  - The wishlist shows wants.
  - "Add" lets you paste a URL or search by title.
  - Production pages show the credits, your outings and the poster.
  - Person pages list every production they appear in and carry the favourite toggle.
- **Routing.** It is hand-written on the History API, with a path pattern mapped to a view.
- **API client.** `api.ts` wraps `fetch`, and `types.ts` mirrors the Go JSON types by hand.
- **PWA.** The manifest makes the app installable. On Android, `share_target` sends a shared page to `/add?url=…`; on iOS, the member pastes the URL instead. A service worker is added only if installation requires one, and none caches anything offline in v1.
- **CSS and design tokens.**
  - Cascade layers: `@layer reset, tokens, base, components;`.
  - `tokens.css` defines primitive values (`--gray-1…12`, `--accent-1…12`, `--space-1…8`, `--text-xs…2xl`, `--radius-*`, `--shadow-*`) and semantic tokens that reference them (`--color-surface`, `--color-text`, `--color-text-muted`, `--color-border`, `--color-accent`, `--space-gap`, ...).
  - Dark mode swaps only the semantic layer through `prefers-color-scheme`.
  - Components use **only semantic tokens**: no raw colour or pixel value outside `tokens.css`.
  - Native CSS nesting is used, with no preprocessor.
  - The layout is mobile-first.

## 10. Security

- **Exposed surface.** Only login, invite checking and redemption, and static assets are public. Everything else requires a session; the import endpoint in particular must never act as an open proxy.
- **Cross-site protection.** `http.CrossOriginProtection` from the standard library covers every mutating request, alongside the SameSite cookie.
- **Passwords.** They are hashed with argon2id.
- **Login limits.** In memory, at most 10 attempts per 15 minutes per IP and per login. Failures are recorded as `login_failed` events.
  - Behind Dokku's nginx, every request comes from the proxy. The client IP is therefore read from `X-Forwarded-For`, and only when `TRUST_PROXY=1` is set. Otherwise the header is ignored so that it can't be spoofed.
- **Headers.** A strict CSP (`default-src 'self'`; images also from `'self'`, since posters are served locally), `X-Content-Type-Options`, `Referrer-Policy`, and HSTS (sent by the app, passed through by nginx).
- **HTTPS.** TLS ends at nginx and the app only listens over HTTP inside the container. Cookies are still `Secure`, since the browser sees HTTPS. The app is never exposed directly: Dokku publishes only nginx.
- **Rendering.** Extracted content is rendered only as text (section 5).
- **Request limits.** Request bodies are size-limited and the server has read and write timeouts.

## 11. Operations

- **Binary.** The `strapontin` binary embeds the migrations and the frontend build. Its subcommands are `serve`, `admin create`, `backup <path>`, `export`, and `reextract`.
- **Configuration**, through environment variables (Dokku: `dokku config:set`), each also available as a flag:

  | Variable | Default | Meaning |
  |---|---|---|
  | `PORT` | `8080` | HTTP listen port |
  | `DATA_DIR` | `./data` locally, `/data` in the image | database and local backups |
  | `TRUST_PROXY` | unset | read the client IP from `X-Forwarded-For` (section 10) |
  | `BASE_URL` | `http://localhost:8080` | used to build invite and reset links |

- **Health check.** `GET /healthz` is public and returns 200 once the database is open. It reveals nothing else.
- **Start locally.** Development and early use run on the laptop with `strapontin serve`, without a container or a domain. The image can be tested locally with:

  ```
  docker build -t strapontin . && docker run -p 8080:8080 -v "$PWD/data:/data" strapontin
  ```

### 11.1 Container image

A multi-stage `Dockerfile` at the repository root:

```dockerfile
# 1. Frontend
FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --ignore-scripts
COPY web/ ./
RUN npm run build                      # writes web/dist

# 2. Binary
FROM golang:1.27-alpine AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /strapontin .

# 3. Runtime: CA certificates, tzdata, non-root user, no shell
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=go /strapontin /strapontin
ENV DATA_DIR=/data PORT=8080
EXPOSE 8080
VOLUME /data
ENTRYPOINT ["/strapontin"]
CMD ["serve"]
```

- **CA certificates** are needed because the app makes outgoing HTTPS requests to venue sites and sources. The distroless image includes them.
- **Pinned base images.** The Go and Node versions are fixed in the Dockerfile. Base images can later be pinned by digest for reproducible builds.
- **`npm ci`** installs exactly what the lockfile says, so the Solid 2 exception (section 9) only matters when the lockfile is created or updated, not when the image is built.
- **`.dockerignore`** excludes `data/`, `web/node_modules/`, `web/dist/`, `research_notes/` and `reports/`.

### 11.2 Dokku deployment

One-time setup on the VPS:

```
dokku apps:create strapontin
dokku storage:ensure-directory strapontin
dokku storage:mount strapontin /var/lib/dokku/data/storage/strapontin:/data
chown -R 65532:65532 /var/lib/dokku/data/storage/strapontin   # distroless "nonroot" user
dokku config:set strapontin TRUST_PROXY=1 BASE_URL=https://<domain>
dokku domains:set strapontin <domain>
dokku ports:set strapontin http:80:8080
dokku letsencrypt:enable strapontin
dokku checks:disable strapontin
```

- **Deploying.** `git push dokku main`. Dokku finds the `Dockerfile` and builds the image on the VPS.
  - If the VPS is too small to build Node and Go comfortably (about 1 GB of RAM), build the image locally, push it to the VPS with `docker save | ssh … docker load`, then run `dokku git:from-image strapontin strapontin:<tag>`.
- **Zero-downtime deploys are disabled** (`checks:disable`). Otherwise Dokku starts the new container while the old one still runs. Two processes would then share the SQLite file, and the new one would migrate the schema under the old code. A restart of a few seconds is acceptable here.
- **The database lives only on the mounted volume.** Nothing is written inside the container, which is replaced on every deploy.
- **The first admin** is created with `dokku run strapontin admin create`. The arguments are passed to the binary's `ENTRYPOINT`.

### 11.3 Backups and migrations

- **Backups.**
  - Inside the app, a daily `VACUUM INTO` writes to `/data/backups/`, keeping 14 copies. They are on the mounted volume, so they survive deploys.
  - A cron job on the VPS host copies `/var/lib/dokku/data/storage/strapontin/backups/` to another machine, for example with `rsync` or `restic`.
  - The admin page shows the date of the last backup.
- **Migrations.** These are `internal/db/migrations/NNNN_name.sql` files applied at startup inside a transaction. They only move forward. A backup is taken just before any migration runs.

## 12. Repository layout

```
main.go                     subcommands, configuration
internal/db/                connection, migrations/, queries
internal/extract/           jsonld.go, microdata.go, opengraph.go, credits.go, sites.go, testdata/
internal/sources/           openagenda.go, tourisme.go, wikidata.go
internal/auth/              sessions, passwords, invites, rate limiting
internal/api/               handlers, middleware
internal/admin/             stats queries
web/                        frontend (section 9); web/dist is embedded
Dockerfile  .dockerignore   container image (section 11.1)
Makefile                    dev, build, test, image
```

## 13. Build order

Each step leaves the app usable.

1. **Foundations.** Schema (catalogue, personal layer, `member`, `session`), CLI `admin create`, login, and manual entry of productions, people, credits and outings. Journal, wishlist and person pages.
2. **URL import.** Fetch, snapshot, structured data (JSON-LD, OpenGraph), draft and correction screen, posters.
3. **Credits.** Text parsing, person matching, golden tests on real snapshots.
4. **Search.** OpenAgenda and the tourism office, plus local FTS.
5. **Opening to others.** Invites, quota and cap, password resets, admin page, events, ntfy. Then choose a domain and deploy to Dokku with Let's Encrypt and off-site backups (section 11.2). **This must be done before the first invitation.** Steps 1–4 need neither a domain nor a server. The `Dockerfile` can come earlier to test the image locally.
6. **Polish.** Wikidata and BnF linking, export and account deletion, PWA manifest and Android share target, `reextract`.

## 14. Decisions and open points

**Decided**

- Solid 2 works with Vite 8 through `@solidjs/vite-plugin` 3 (section 9).
- `invite_quota` = 1.
- No OpenAgenda API key; the keyless endpoint is used and isolated (section 6).
- No domain and no server to start with.
- Distributed as a container image and served by Dokku on your existing VPS. TLS and the domain are handled by Dokku, not the app (section 11).

**Open**

- Choose a domain name, at build step 5.
- Check the VPS has enough RAM to build the image on the server, or build locally (section 11.2).
- Read the terms of use of the sites imported most often (the Toulouse report did not review them).
