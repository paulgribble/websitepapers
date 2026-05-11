# websitepapers

A minimal Go web application for collecting and browsing academic papers by DOI. Paste a DOI (or DOI URL), and the app fetches metadata from the [Crossref API](https://www.crossref.org/) and stores it in a local SQLite database. Browse your library in the web UI, delete papers you no longer want, and export the lot as Markdown or BibTeX.

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

- Go 1.25+
- A C compiler (gcc/clang) — required for the `mattn/go-sqlite3` driver (cgo)

`golang.org/x/text` is fetched automatically by `go mod tidy` / `go build`.

## Quick start

```bash
git clone https://github.com/<your-username>/websitepapers.git
cd websitepapers
make run
```

Then open <http://localhost:8080>.

A `dois.db` SQLite file is created in the working directory on first run.

## Make targets

| Target       | Action                                |
| ------------ | ------------------------------------- |
| `make run`   | `go run .`                            |
| `make build` | Compile the `doi-app` binary          |
| `make test`  | `go test ./...`                       |
| `make fmt`   | `gofmt -w .` + `go vet ./...`         |
| `make clean` | Remove the built binary               |

## Routes

| Method |     Path      |                  Description                  |
| ------ | ------------- | --------------------------------------------- |
| GET    | `/`           | List all papers (newest first)                |
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

## Project layout

```
main.go        — entry point: types, main(), renderTemplate
handlers.go    — HTTP handlers + citation formatting
db.go          — schema, migrations, CRUD
doi.go         — DOI regex + normalizer
crossref.go    — Crossref client + JSON schema
bibtex.go      — BibTeX export helpers
main_test.go   — table-driven tests
index.html     — single HTML template (UI)
Makefile       — build / run / fmt / clean targets
go.mod         — module definition
dois.db        — SQLite database (created at runtime)
LICENSE        — project license
```

## License

MIT
