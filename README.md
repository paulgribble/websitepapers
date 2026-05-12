# websitepapers

A minimal Python web application for collecting and browsing academic papers by DOI. Paste a DOI (or DOI URL), and the app fetches metadata from the [Crossref API](https://www.crossref.org/) and stores it in a local SQLite database. Browse your library in the web UI, delete papers you no longer want, and export the lot as Markdown or BibTeX.

## Features

- Paste a bare DOI (`10.xxxx/...`) or any common URL form (`https://doi.org/...`, `dx.doi.org/...`)
- Automatic metadata lookup via Crossref (title, authors, journal, year, volume, pages); identifies the client to Crossref's polite pool via `User-Agent`
- Multi-initial parsing of given names (e.g. `Andrew A.G. Mattar`, `Marie-Claude`, Unicode like `Émile`)
- Case-insensitive duplicate detection (DOIs are officially case-insensitive)
- Browse all papers in a styled table, newest first; per-row ✕ delete button
- One-click export to Markdown (`papers.md`)
- One-click export to BibTeX (`papers.bib`) with proper `Family, Given` author format and ASCII-folded citation keys
- Preprint-aware journal-name resolution and citation formatting for bioRxiv / medRxiv

## Requirements

- Python 3.10+
- [`uv`](https://docs.astral.sh/uv/) for dependency management

Flask (and pytest for development) are installed automatically into a project-local `.venv/`. No C compiler is required — the SQLite driver is stdlib.

## Quick start

```bash
git clone https://github.com/<your-username>/websitepapers.git
cd websitepapers
make install
make run
```

Then open <http://localhost:8080>.

A `dois.db` SQLite file is created in the working directory on first run.

## Make targets

|        Target         |                            Action                            |
| --------------------- | ------------------------------------------------------------ |
| `make install`        | `uv sync` (creates `.venv`, installs deps)                   |
| `make run`            | `uv run python app.py`                                       |
| `make test`           | `uv run pytest`                                              |
| `make clean`          | Remove `.venv`, `__pycache__`, `.pytest_cache`, `uv.lock`    |
| `make docker-build`   | Build the image locally as `websitepapers:latest`            |
| `make docker-run`     | Run the locally built image on port 80, mounting `./storage` |
| `make docker-pull`    | Pull the published image from GHCR                           |
| `make docker-run-ghcr`| Run the GHCR image on port 80, mounting `./storage`          |

## Routes

| Method |     Path      |                  Description                  |
| ------ | ------------- | --------------------------------------------- |
| GET    | `/`           | List all papers (newest first)                |
| GET    | `/up`         | Health check (returns 200 `OK`) — used by the Docker HEALTHCHECK |
| POST   | `/submit`     | Validate DOI, fetch metadata, store in DB     |
| POST   | `/delete`     | Delete a row by `id`                          |
| GET    | `/export`     | Download `papers.md` — all papers as Markdown |
| GET    | `/export.bib` | Download `papers.bib` — all papers as BibTeX  |

## Markdown export

Each paper is written to `papers.md` as:

```markdown
### 1
**Title**  
Authors (Year)  
[Citation](https://doi.org/DOI)
```

(The `**Title**` and `Authors (Year)` lines end with two trailing spaces to produce hard line breaks.)

The citation text adapts to what Crossref returns:

- **bioRxiv / medRxiv**: `bioRxiv:2026.04.27.721195`
- **Volume + Pages**: `J Neurophysiol 135:1175–1185`
- **Volume only**: `J Neurophysiol 135`
- **Pages only**: `J Neurophysiol 1175–1185`
- **Journal only**: bare journal name
- **No journal**: bare DOI

## BibTeX export

Each paper is written to `papers.bib` as a standard `@article{…}` block:

```bibtex
@article{smith2024demo,
  author  = {Smith, J. and Jones, A.},
  title   = {{Demo Title}},
  journal = {J Demo},
  year    = {2024},
  volume  = {42},
  pages   = {1-10},
  doi     = {10.xxxx/yyyy}
}
```

Citation keys are `firstAuthorSurname + year + firstAlphanumTitleWord` (non-alphanumerics stripped), ASCII-folded so authors like *Müller* get key `muller…` rather than `mller…`. Collisions within a single export get `_2`, `_3` suffixes.

## Database schema

A single `papers` table is created (and lightly migrated) on startup:

```sql
CREATE TABLE papers (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  doi      TEXT UNIQUE,
  title    TEXT,
  authors  TEXT,
  journal  TEXT,
  pub_date TEXT,    -- year
  volume   TEXT,
  page     TEXT
);
```

## Docker

A prebuilt image is published to the GitHub Container Registry on every push to `main` and every `v*.*.*` tag:

```
ghcr.io/paulgribble/websitepapers:latest
ghcr.io/paulgribble/websitepapers:<git-sha>
ghcr.io/paulgribble/websitepapers:<version>   # on v*.*.* tags
```

Pull and run:

```bash
docker pull ghcr.io/paulgribble/websitepapers:latest
mkdir -p ./storage
docker run --rm -p 80:80 \
  -v "$PWD/storage:/storage" \
  --name websitepapers \
  ghcr.io/paulgribble/websitepapers:latest
```

Or via the Makefile: `make docker-pull && make docker-run-ghcr`.

The SQLite database lives at `/storage/dois.db` inside the container; mount any host directory to `/storage` to persist it. `DB_PATH` can override the path if needed. A `GET /up` endpoint returns `200 OK` for health checks (the image's `HEALTHCHECK` uses it).

To build locally instead of pulling: `make docker-build && make docker-run`.

### Using with ONCE

The image is compatible with [ONCE](https://github.com/basecamp/once) (Basecamp's self-hosting platform): it serves HTTP on port 80, exposes a `/up` healthcheck, and persists data in `/storage`. To install, run `once` and paste `ghcr.io/paulgribble/websitepapers:latest` at the image-path prompt.

## Authentication

The app supports optional HTTP Basic Auth via two environment variables:

```
BASIC_AUTH_USER=<your-username>
BASIC_AUTH_PASS=<your-password>
```

- If **both** are set, every route except `/up` requires the credentials. Your browser will prompt on first visit.
- If **either** is unset, auth is disabled (this is the default — fine for `make run` on `localhost`).
- The `/up` healthcheck stays open so the Docker `HEALTHCHECK` and ONCE's monitoring continue to work without credentials.

For ONCE deployments, set these in the app's environment via the ONCE admin UI — the password lives in ONCE's secret store, not in the image or your repo. There is no signup flow and no per-user accounts; it's a single shared credential intended for one-person libraries.

## Project layout

```
app.py                — Flask app, routes, citation_text
wsgi.py               — Gunicorn entry point (calls init_db at import)
db.py                 — Paper dataclass, sqlite ops
doi.py                — DOI regex + normalizer
crossref.py           — Crossref client + given_initials
bibtex.py             — BibTeX export helpers
test_app.py           — pytest tests
templates/index.html  — Jinja2 template (UI)
pyproject.toml        — project metadata + deps
uv.lock               — dependency lockfile (generated by uv)
Makefile              — install / run / test / clean / docker-*
Dockerfile            — multi-stage build serving via Gunicorn on port 80
.dockerignore         — files excluded from the Docker build context
dois.db               — SQLite database (created at runtime)
LICENSE               — project license
```

## License

MIT
