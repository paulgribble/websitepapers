# websitepapers

A minimal Go web application for collecting and browsing academic papers by DOI. Users paste a DOI or DOI URL, the app fetches metadata from the Crossref API, and persists it to a local SQLite database. Papers can be browsed in a styled table, removed via a per-row delete button, and exported as Markdown or BibTeX.

## Tech stack

- **Language**: Go 1.23.3
- **Web server**: stdlib `net/http`
- **Templating**: stdlib `html/template` (parsed once at startup into a global `tmpl`)
- **Database**: SQLite via `github.com/mattn/go-sqlite3` (requires gcc/cgo)
- **External API**: Crossref (`https://api.crossref.org/works/{doi}`), 10s client timeout, polite-pool `User-Agent` header
- **Unicode**: `golang.org/x/text` (NFD normalization for ASCII-folding citation keys)

## Directory structure

```
main.go        — entry point: types, package vars, main(), renderTemplate
handlers.go    — HTTP handlers + citationText
db.go          — openDB, runMigration, getPapers, paperExists, insertPaper, deletePaper
doi.go         — doiRegex, normalizeDOI
crossref.go    — crossrefBase, crossrefResponse, fetchMetadata, givenInitials
bibtex.go      — writeBibEntry, bibKey, bibAlpha, bibAsciiFold, bibAuthors, bibEscape, isInitial
main_test.go   — table-driven tests + httptest-mocked Crossref helpers
index.html     — single HTML template (UI)
go.mod         — module definition (module: doi-app)
go.sum         — dependency lock
dois.db        — SQLite database (created at runtime)
.gitignore     — ignores the doi-app build binary
```

All six `.go` source files share `package main`, so no import boilerplate between them.

## How to run / test

```bash
go run main.go            # server starts at http://localhost:8080
go test ./...             # run all unit tests
go vet ./...              # static analysis
```

## Routes

| Method |     Path      |                        Description                        |
| ------ | ------------- | --------------------------------------------------------- |
| GET    | `/`           | List all papers (newest first); 405 on non-GET            |
| POST   | `/submit`     | Validate DOI, fetch metadata, store in DB; non-POST → `/` |
| POST   | `/delete`     | Delete row by `id` form field; non-POST → `/`             |
| GET    | `/export`     | Download `papers.md` — all papers as Markdown             |
| GET    | `/export.bib` | Download `papers.bib` — all papers as BibTeX              |

Error paths return real HTTP status codes (400 invalid DOI, 409 duplicate, 500 db/insert/delete, 502 Crossref upstream failure) instead of always 200.

## Data model

```go
type Paper struct {
    ID                                                int
    DOI, Title, Authors, Journal, Year, Volume, Pages string
}

type PageData struct {
    Papers  []Paper
    Message string  // shown as a red error/info banner above the form
}
```

`Paper.ID` is the SQLite rowid; the per-row delete form posts it back as a hidden field.

## Key functions, by file

### main.go
|                Function                |                              Purpose                              |
| -------------------------------------- | ----------------------------------------------------------------- |
| `main()`                               | Parse template, open/migrate DB, register routes, listen on :8080 |
| `renderTemplate(w, status, msg, list)` | Set status code, then execute template with `PageData`            |

### handlers.go
|      Function       |                                     Purpose                                     |
| ------------------- | ------------------------------------------------------------------------------- |
| `handleHome()`      | Render paper list (GET only; 405 otherwise)                                     |
| `handleSubmit()`    | Normalize DOI → validate → dedupe → fetch → insert; 303 → `/`                   |
| `handleDelete()`    | Delete by form `id`, 303 → `/`                                                  |
| `handleExport()`    | Stream `papers.md` (`text/markdown; charset=utf-8`)                             |
| `handleExportBib()` | Stream `papers.bib` (`application/x-bibtex; charset=utf-8`) via `writeBibEntry` |
| `citationText(p)`   | Display string for the markdown export's citation link (preprint-aware)         |

### db.go
|           Function           |                                        Purpose                                        |
| ---------------------------- | ------------------------------------------------------------------------------------- |
| `openDB(path)`               | Open SQLite, create schema, run migrations                                            |
| `runMigration(d, name, sql)` | Run one migration; logs only unexpected errors (skips SQLite "duplicate column name") |
| `getPapers()`                | `SELECT … ORDER BY id DESC` with `COALESCE(volume,'')`, `COALESCE(page,'')`           |
| `paperExists(doi)`           | `SELECT EXISTS(...)` for duplicate check                                              |
| `insertPaper(p)`             | INSERT one row                                                                        |
| `deletePaper(id)`            | DELETE WHERE id=?                                                                     |

### doi.go
|     Function      |                      Purpose                      |
| ----------------- | ------------------------------------------------- |
| `normalizeDOI(s)` | Trim, lowercase, strip known doi.org URL prefixes |
| `doiRegex`        | `(?i)^10\.\d{4,}(?:\.\d+)?/\S+$`                  |

### crossref.go
|       Function       |                                             Purpose                                              |
| -------------------- | ------------------------------------------------------------------------------------------------ |
| `fetchMetadata(doi)` | GET Crossref, parse JSON into `Paper`                                                            |
| `givenInitials(s)`   | One initial per letter-run: `"Andrew A.G."` → `"A. A. G."`                                       |
| `crossrefResponse`   | Named type for the JSON payload (lifted from inline struct)                                      |
| `crossrefBase`       | Package var (default `https://api.crossref.org`) — overridden by tests via `httptest.Server.URL` |

### bibtex.go
|          Function           |                                                            Purpose                                                            |
| --------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| `writeBibEntry(w, p, used)` | Emit one `@article{…}` block; tracks key collisions in `used`                                                                 |
| `bibKey(p, used)`           | `firstAuthor + year + firstTitleWord`, lowercased, with `_2`, `_3` collision suffixes                                         |
| `bibAlpha(s)`               | Keep ASCII letters/digits after ASCII-folding                                                                                 |
| `bibAsciiFold(s)`           | NFD-normalize and strip combining marks (`Müller` → `Muller`)                                                                 |
| `bibAuthors(s)`             | Convert stored `"Family I."` → BibTeX `"Family, I."`; splits at first initial-shaped token so multi-word surnames stay intact |
| `bibEscape(s)`              | Escape `\ & % _ # $ { }` for TeX                                                                                              |
| `isInitial(t)`              | Predicate: `<rune>.`                                                                                                          |

## Database schema

```sql
CREATE TABLE papers (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  doi      TEXT UNIQUE,
  title    TEXT,
  authors  TEXT,
  journal  TEXT,
  pub_date TEXT,    -- stores Year (column name predates schema cleanup)
  volume   TEXT,    -- added by migration on startup
  page     TEXT     -- added by migration on startup
);
```

Three idempotent migrations run on every startup via `runMigration`:

1. `ALTER TABLE papers ADD COLUMN volume TEXT`
2. `ALTER TABLE papers ADD COLUMN page TEXT`
3. `UPDATE papers SET doi = LOWER(doi) WHERE doi != LOWER(doi)`

The first two error with "duplicate column name" once the columns exist; that error is silently swallowed. Anything else is logged.

## Crossref metadata parsing (`fetchMetadata` in crossref.go)

- Polite-pool: every request sets `User-Agent: websitepapers/0.1 (mailto:pgribble@uwo.ca)`.
- Schema is parsed into a named `crossrefResponse` type at file scope (rather than buried inline).
- **Title**: first element of `message.title[]`
- **Journal**: first element of `message.container-title[]`; falls back to `message.institution[0].name` for preprints (bioRxiv/medRxiv have empty `container-title`)
- **Volume**: `message.volume`
- **Pages**: `message.page`; if both volume and pages are empty, falls back to `message.article-number`
- **Authors**: joined `"Family I. [I. ...]"` via `, `, where each given name produces one initial per letter-run (handles `"Paul L"`, `"Paul L."`, `"Andrew A.G."`, hyphenated `"Marie-Claude"`, Unicode `"Émile"`).
- **Year**: first available year from `published-print` → `published-online` → `issued` (`date-parts[0][0]`)
- Non-200 responses return `"DOI not found (status N)"`

## DOI handling

- Accepted input: bare DOI (`10.xxxx/...`) or URL variants (`https://doi.org/...`, `https://dx.doi.org/...`, `doi.org/...`, `dx.doi.org/...`)
- Validation regex: `(?i)^10\.\d{4,}(?:\.\d+)?/\S+$`
- `normalizeDOI()` trims, strips known URL prefixes, **and lowercases** the result. DOIs are officially case-insensitive per the DOI handbook.
- Duplicate detection: `SELECT EXISTS(SELECT 1 FROM papers WHERE doi=?)` against the lowercase form.
- A one-time idempotent startup migration (`UPDATE … LOWER(doi) WHERE doi != LOWER(doi)`) lowercases any pre-existing rows.
- Crossref API URL: `strings.TrimRight(crossrefBase, "/") + "/works/" + url.PathEscape(doi)` — defends against a trailing slash on the base and safely encodes special characters in the path.

## UI (`index.html`)

- Single page: error banner (when `.Message`), DOI input form with **Fetch & Add** + **Export MD** + **Export BibTeX** buttons, then a styled table.
- Table columns: **Paper Details** (title + monospace `https://doi.org/...` link), **Authors**, **Journal / Date** (formats `Journal Volume:Pages` inline; falls back to `Journal Pages` then `Journal`; year on second line), and an actions column with a per-row ✕ delete button (POSTs to `/delete` guarded by a JS `confirm()`).
- Empty state: `"Library is empty."` row.

## Markdown export (`papers.md`)

Each paper is rendered as:

```markdown
### 1
**Title**  
Authors (Year)  
[Citation](https://doi.org/DOI)
```

The `Authors (Year)` line drops the `(Year)` suffix entirely when `Year` is empty.

`citationText()` formats the citation link text as:
- **bioRxiv / medRxiv** (case-insensitive journal match): `Journal:doi_suffix` (e.g. `bioRxiv:2026.04.27.721195`)
- **Volume + Pages**: `Journal Volume:Pages` (e.g. `J Neurophysiol 135:1175–1185`)
- **Volume only**: `Journal Volume`
- **Pages only**: `Journal Pages`
- **Journal only** (no volume/pages): bare journal name
- **No journal**: bare DOI

## BibTeX export (`papers.bib`)

Each paper is emitted as an `@article{…}` block:

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

- **Citation key**: `firstAuthorSurname + year + firstAlphanumTitleWord`, lowercased, ASCII-folded (`Müller` → `muller`). Collisions get `_2`, `_3` suffixes within a single export.
- **Author format**: `"Family, Given"` (the only form BibTeX parses correctly). The `bibAuthors` split picks the first initial-shaped token (`<rune>.`) so multi-word surnames (`van der Berg P. L.`) stay intact.
- **Title**: wrapped in `{{...}}` to protect capitalization in styles that lowercase by default.
- **Field escaping**: `\ & % _ # $ { }` are TeX-escaped for title/journal/volume/pages. The `doi` field is intentionally not escaped — modern bibliography styles treat it as a verbatim identifier.
- Empty fields are omitted.

## Tests

`main_test.go` provides table-driven tests. Two helpers — `mockCrossrefHandler` and `mockCrossrefBody` — point `crossrefBase` at an `httptest.NewServer` and restore it via `t.Cleanup`. Coverage:

- `TestNormalizeDOI` — every prefix variant, casing, whitespace
- `TestCitationText` — bioRxiv/medRxiv special case + every volume/pages combo
- `TestFetchMetadata` — happy path (asserts UA), 404, article-number fallback, year priority fallthrough, bioRxiv institution fallback, trailing-slash on base
- `TestGivenInitials` — single, multi, smashed (`A.G.`), hyphenated, Unicode
- `TestBibAuthors` — single + multi-author, multi-word surnames, multi-initial
- `TestBibEscape` — every escaped char
- `TestBibAsciiFold` — Müller, Émile, Zoë, naïve, Çelik
- `TestBibKey` — collision suffixes + diacritic input
