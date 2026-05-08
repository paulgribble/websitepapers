# websitepapers

A minimal Go web application for collecting and browsing academic papers by DOI. Users paste a DOI or DOI URL, the app fetches metadata from the Crossref API, and persists it to a local SQLite database for browsing. Papers can be exported as a formatted Markdown file.

## Tech stack

- **Language**: Go 1.23.3
- **Web server**: stdlib `net/http`
- **Templating**: stdlib `html/template` (template parsed once at startup into a global `tmpl`)
- **Database**: SQLite via `github.com/mattn/go-sqlite3` (requires gcc/cgo)
- **External API**: Crossref (`https://api.crossref.org/works/{doi}`), 10s client timeout

## Directory structure

```
main.go       — server, handlers, business logic
index.html    — single HTML template (UI)
go.mod        — module definition (module: doi-app)
go.sum        — dependency lock
dois.db       — SQLite database (created at runtime)
```

## How to run

```bash
go run main.go
# Server starts at http://localhost:8080
```

## Routes

| Method |   Path    |                        Description                        |
| ------ | --------- | --------------------------------------------------------- |
| GET    | `/`       | List all papers (newest first)                            |
| POST   | `/submit` | Validate DOI, fetch metadata, store in DB; non-POST → `/` |
| GET    | `/export` | Download `papers.md` — all papers as Markdown             |

## Data model

```go
type Paper struct {
    DOI, Title, Authors, Journal, Year, Volume, Pages string
}
type PageData struct {
    Papers  []Paper
    Message string  // shown as a red error/info banner above the form
}
```

## Key functions in main.go

|      Function      |                                    Purpose                                     |
| ------------------ | ------------------------------------------------------------------------------ |
| `main()`           | Parse template, open/migrate DB, register routes, start :8080                  |
| `handleHome()`     | Render paper list                                                              |
| `handleSubmit()`   | Normalize DOI, validate, check duplicate, fetch metadata, insert, 303 → `/`    |
| `handleExport()`   | Stream `papers.md` download (`Content-Type: text/markdown; charset=utf-8`)     |
| `fetchMetadata()`  | GET Crossref, parse title/authors/journal/volume/pages/year                    |
| `citationText()`   | Build citation display string (preprint-aware)                                 |
| `normalizeDOI()`   | Strip doi.org/dx.doi.org URL prefixes (case-insensitive); preserves DOI casing |
| `getPapers()`      | `SELECT ... ORDER BY id DESC` with `COALESCE(volume,'')`, `COALESCE(page,'')`  |
| `renderTemplate()` | Execute global template with `PageData`                                        |

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

The two `ALTER TABLE ADD COLUMN` migrations run on every startup and are silently ignored once the columns exist.

## Crossref metadata parsing (`fetchMetadata`)

- **Title**: first element of `message.title[]`
- **Journal**: first element of `message.container-title[]`
- **Volume**: `message.volume`
- **Pages**: `message.page`; if both volume and pages are empty, falls back to `message.article-number`
- **Authors**: joined `"Family G."` (family name + first initial of given name) via `, `
- **Year**: first available year from `published-print` → `published-online` → `issued` (`date-parts[0][0]`)
- Non-200 responses return `"DOI not found (status N)"`

## UI (`index.html`)

- Single page: error banner (when `.Message`), DOI input form with **Fetch & Add** + **Export** buttons, then a styled table.
- Table columns: **Paper Details** (title + monospace `https://doi.org/...` link), **Authors**, **Journal / Date** (formats `Journal Volume:Pages` inline; falls back to `Journal Pages` then `Journal`; year on second line).
- Empty state: `"Library is empty."` row.

## Export format (papers.md)

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

## DOI handling

- Accepted input: bare DOI (`10.xxxx/...`) or URL variants (`https://doi.org/...`, `https://dx.doi.org/...`, `doi.org/...`, `dx.doi.org/...`)
- Validation regex: `(?i)^10\.\d{4,}(?:\.\d+)?/\S+$`
- `normalizeDOI()` strips known URL prefixes via case-insensitive `HasPrefix`, but slices the original input so DOI casing is preserved
- Duplicate detection: `SELECT EXISTS(SELECT 1 FROM papers WHERE doi=?)` (exact match — DOIs are not lowercased before storage)
- Crossref API URL built with `url.URL{Path: "/works/" + doi}` to safely encode special characters
