# websitepapers

A minimal Go web application for collecting and browsing academic papers by DOI. Paste a DOI (or DOI URL), and the app fetches metadata from the [Crossref API](https://www.crossref.org/) and stores it in a local SQLite database. Browse your library in the web UI, or export it as a formatted Markdown file.

## Features

- Paste a bare DOI (`10.xxxx/...`) or any common URL form (`https://doi.org/...`, `dx.doi.org/...`)
- Automatic metadata lookup via Crossref (title, authors, journal, year, volume, pages)
- Duplicate detection on submit
- Browse all papers in a styled table, newest first
- One-click export to a Markdown bibliography (`papers.md`)
- Preprint-aware citation formatting for bioRxiv / medRxiv

## Requirements

- Go 1.23+
- A C compiler (gcc/clang) — required for the `mattn/go-sqlite3` driver (cgo)

## Quick start

```bash
git clone https://github.com/<your-username>/websitepapers.git
cd websitepapers
go run main.go
```

Then open <http://localhost:8080>.

A `dois.db` SQLite file is created in the working directory on first run.

### Using the Makefile

```bash
make build   # build the doi-app binary
make run     # go run main.go
make fmt     # gofmt + go vet
make clean   # remove the built binary
```

## Routes

| Method |   Path    |                  Description                  |
| ------ | --------- | --------------------------------------------- |
| GET    | `/`       | List all papers (newest first)                |
| POST   | `/submit` | Validate DOI, fetch metadata, store in DB     |
| GET    | `/export` | Download `papers.md` — all papers as Markdown |

## Export format

Each paper is written to `papers.md` as:

```markdown
### 1
**Title**
Authors (Year)
[Citation](https://doi.org/DOI)
```

The citation text adapts to what Crossref returns:

- **bioRxiv / medRxiv**: `bioRxiv:2026.04.27.721195`
- **Volume + Pages**: `J Neurophysiol 135:1175–1185`
- **Volume only**: `J Neurophysiol 135`
- **Pages only**: `J Neurophysiol 1175–1185`
- **Journal only**: bare journal name
- **No journal**: bare DOI

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

## Project layout

```
main.go       — server, handlers, business logic
index.html    — single HTML template (UI)
Makefile      — build / run / fmt / clean targets
go.mod        — module definition
dois.db       — SQLite database (created at runtime)
```

## License

MIT
