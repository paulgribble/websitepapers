# websitepapers

A minimal Python web application for collecting and browsing academic papers by DOI. Users paste a DOI or DOI URL, the app fetches metadata from the Crossref API, and persists it to a local SQLite database. Papers can be browsed in a styled table, removed via a per-row delete button, and exported as Markdown or BibTeX.

The original Go implementation is preserved verbatim in `old_go/` (no longer maintained).

## Tech stack

- **Language**: Python 3.10+
- **Web framework**: Flask 3.x
- **Templating**: Jinja2 (bundled with Flask)
- **Database**: SQLite via stdlib `sqlite3` (no cgo, no compiler needed)
- **External API**: Crossref (`https://api.crossref.org/works/{doi}`), 10s urllib timeout, polite-pool `User-Agent` header
- **HTTP client**: stdlib `urllib.request`
- **Unicode**: stdlib `unicodedata` (NFD normalization for ASCII-folding citation keys)
- **Package manager**: `uv` (project mode via `pyproject.toml`; no `requirements.txt`)

## Directory structure

```
app.py                — Flask app, routes, citation_text, main entry point
db.py                 — Paper dataclass, init_db, get/insert/delete/exists
doi.py                — DOI_REGEX, normalize_doi
crossref.py           — fetch_metadata, given_initials, CROSSREF_BASE
bibtex.py             — write_bib_entry, bib_key, bib_alpha, bib_ascii_fold,
                        bib_authors, bib_escape
test_app.py           — pytest tests (62 cases, mostly @pytest.mark.parametrize)
templates/index.html  — single Jinja2 template (UI)
pyproject.toml        — project metadata + Flask (dependency-group dev: pytest)
uv.lock               — uv's dependency lockfile (generated)
Makefile              — install / run / test / clean
dois.db               — SQLite database (created at runtime)
old_go/               — archived Go implementation (do not edit)
README.md             — user-facing project README
LICENSE               — project license
```

## How to run / test

```bash
make install              # uv sync (creates .venv, installs deps)
make run                  # uv run python app.py → http://localhost:8080
make test                 # uv run pytest
make clean                # remove .venv, __pycache__, .pytest_cache, uv.lock
```

Direct invocation also works: `uv run python app.py`, `uv run pytest`.

## Routes

| Method |     Path      |                        Description                        |
| ------ | ------------- | --------------------------------------------------------- |
| GET    | `/`           | List all papers (newest first); 405 on non-GET            |
| POST   | `/submit`     | Validate DOI, fetch metadata, store in DB; non-POST → `/` |
| POST   | `/delete`     | Delete row by `id` form field; non-POST → `/`             |
| GET    | `/export`     | Download `papers.md` — all papers as Markdown             |
| GET    | `/export.bib` | Download `papers.bib` — all papers as BibTeX              |

Error paths return real HTTP status codes (400 invalid DOI, 409 duplicate, 500 db/insert/delete, 502 Crossref upstream failure).

## Data model

```python
@dataclass
class Paper:
    id: int = 0
    doi: str = ""
    title: str = ""
    authors: str = ""
    journal: str = ""
    year: str = ""
    volume: str = ""
    pages: str = ""
```

Field order matches the SELECT column order in `get_papers`, so `Paper(*row)` works positionally. The template uses lowercase attribute access (`{{ p.title }}`).

## Key functions, by file

### app.py
|            Function             |                                     Purpose                                      |
| ------------------------------- | -------------------------------------------------------------------------------- |
| `home()`                        | Render paper list (GET only)                                                     |
| `submit()`                      | Normalize DOI → validate → dedupe → fetch → insert; 303 → `/`                    |
| `delete()`                      | Delete by form `id`, 303 → `/`                                                   |
| `export_md()`                   | Stream `papers.md` (`text/markdown; charset=utf-8`)                              |
| `export_bib()`                  | Stream `papers.bib` (`application/x-bibtex; charset=utf-8`)                      |
| `citation_text(p)`              | Display string for the markdown export's citation link (preprint-aware)          |
| `render_page(status, message)`  | Render `index.html` with current papers list                                     |
| `respond_err(status, msg, err)` | Log err and render error page in one call (analogue of Go's `respondErr` helper) |

The `Content-Type` for both export routes is set via the `headers=` argument rather than `mimetype=`; Flask appends `; charset=utf-8` to `text/*` mimetypes, which would double the charset on `text/markdown; charset=utf-8`.

### db.py
|      Function       |                                      Purpose                                      |
| ------------------- | --------------------------------------------------------------------------------- |
| `init_db()`         | Create schema, run migrations; called from `if __name__ == "__main__":` in app.py |
| `get_papers()`      | `SELECT … ORDER BY id DESC` with `COALESCE(volume,'')`, `COALESCE(page,'')`       |
| `paper_exists(doi)` | `SELECT EXISTS(...)` for duplicate check                                          |
| `insert_paper(p)`   | INSERT one row                                                                    |
| `delete_paper(id)`  | DELETE WHERE id=?                                                                 |

Each function opens its own connection via `_connect()`; SQLite is per-call, not pooled. Fine for a single-user local app.

### doi.py
|      Function      |                      Purpose                      |
| ------------------ | ------------------------------------------------- |
| `normalize_doi(s)` | Trim, lowercase, strip known doi.org URL prefixes |
| `DOI_REGEX`        | `^10\.\d{4,}(?:\.\d+)?/\S+$` (IGNORECASE)         |

### crossref.py
|       Function        |                                                    Purpose                                                    |
| --------------------- | ------------------------------------------------------------------------------------------------------------- |
| `fetch_metadata(doi)` | GET Crossref via urllib, parse JSON into `Paper`                                                              |
| `given_initials(s)`   | One initial per letter-run: `"Andrew A.G."` → `"A. A. G."`                                                    |
| `CROSSREF_BASE`       | Module-level constant `https://api.crossref.org` — monkey-patched by tests via `crossref.CROSSREF_BASE = ...` |

The Crossref response is parsed as a plain `dict`. Missing keys yield empty strings via `dict.get(...) or fallback`. No typed schema (was a nested anonymous struct in the Go version).

### bibtex.py
|            Function             |                                                                                   Purpose                                                                                   |
| ------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `write_bib_entry(out, p, used)` | Emit one `@article{…}` block to a writable text stream; tracks key collisions in `used`                                                                                     |
| `bib_key(p, used)`              | `firstAuthor + year + firstTitleWord`, lowercased, with `_2`, `_3` collision suffixes                                                                                       |
| `bib_alpha(s)`                  | Keep ASCII letters/digits after ASCII-folding                                                                                                                               |
| `bib_ascii_fold(s)`             | NFD-normalize and strip combining marks (`Müller` → `Muller`)                                                                                                               |
| `bib_authors(s)`                | Convert stored `"Family I."` → BibTeX `"Family, I."`; splits at first initial-shaped token so multi-word surnames stay intact                                               |
| `bib_escape(s)`                 | **Single-pass** escape via per-character dict lookup. Sequential `str.replace` would re-escape the braces inside `\textbackslash{}`; the test suite catches this regression |
| `_is_initial(t)`                | Predicate: `<rune>.`                                                                                                                                                        |

## Database schema

Unchanged from the Go version — the existing `dois.db` migrates cleanly:

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

Three idempotent migrations run in `init_db()`:

1. `ALTER TABLE papers ADD COLUMN volume TEXT`
2. `ALTER TABLE papers ADD COLUMN page TEXT`
3. `UPDATE papers SET doi = LOWER(doi) WHERE doi != LOWER(doi)`

The first two raise `sqlite3.OperationalError("duplicate column name")` once the columns exist; only that specific message is swallowed. Anything else is re-raised.

## Crossref metadata parsing (`fetch_metadata` in crossref.py)

- Polite-pool: every request sets `User-Agent: websitepapers/0.1 (mailto:pgribble@uwo.ca)`.
- The response is decoded into a plain `dict` and accessed with `dict.get(...) or fallback` chains.
- **Title**: first element of `message.title[]`
- **Journal**: first element of `message.container-title[]`; falls back to `message.institution[0].name` for preprints (bioRxiv/medRxiv have empty `container-title`)
- **Volume**: `message.volume`
- **Pages**: `message.page`; if both volume and pages are empty, falls back to `message.article-number`
- **Authors**: joined `"Family I. [I. ...]"` via `, `, where each given name produces one initial per letter-run (handles `"Paul L"`, `"Paul L."`, `"Andrew A.G."`, hyphenated `"Marie-Claude"`, Unicode `"Émile"`).
- **Year**: first available year from `published-print` → `published-online` → `issued` (`date-parts[0][0]`)
- Non-200 responses raise `RuntimeError("DOI not found (status N)")` — urllib raises `HTTPError` on 4xx/5xx, which is caught and translated.

## DOI handling

Identical behavior to the Go version:

- Accepted input: bare DOI (`10.xxxx/...`) or URL variants (`https://doi.org/...`, `https://dx.doi.org/...`, `doi.org/...`, `dx.doi.org/...`)
- Validation regex: `^10\.\d{4,}(?:\.\d+)?/\S+$` (case-insensitive)
- `normalize_doi()` trims, strips known URL prefixes, **and lowercases** the result. DOIs are officially case-insensitive per the DOI handbook.
- Duplicate detection: `SELECT EXISTS(SELECT 1 FROM papers WHERE doi=?)` against the lowercase form.
- Crossref API URL: `CROSSREF_BASE.rstrip("/") + "/works/" + urllib.parse.quote(doi, safe='')` — defends against a trailing slash on the base and percent-encodes special characters in the path.

## UI (`templates/index.html`)

Identical layout to the Go version with Jinja2 syntax in place of Go's `html/template`:

- `{{if .Message}}` → `{% if message %}`
- `{{range .Papers}}…{{else}}…{{end}}` → `{% for p in papers %}…{% else %}…{% endfor %}`
- `{{.Title}}` → `{{ p.title }}` (lowercase attribute access for the dataclass)

CSS, copy, error banner, form (Fetch & Add + Export MD + Export BibTeX), table columns, and the per-row ✕ delete button are unchanged.

## Markdown export (`papers.md`)

Each paper is rendered as:

```markdown
### 1
**Title**  
Authors (Year)  
[Citation](https://doi.org/DOI)
```

The `Authors (Year)` line drops the `(Year)` suffix entirely when `Year` is empty.

`citation_text()` formats the citation link text as:
- **bioRxiv / medRxiv** (case-insensitive journal match): `Journal:doi_suffix` (e.g. `bioRxiv:2026.04.27.721195`)
- **Volume + Pages**: `Journal Volume:Pages` (e.g. `J Neurophysiol 135:1175–1185`)
- **Volume only**: `Journal Volume`
- **Pages only**: `Journal Pages`
- **Journal only** (no volume/pages): bare journal name
- **No journal**: bare DOI

## BibTeX export (`papers.bib`)

Each paper is emitted as an `@article{…}` block; the format and rules (citation key, author format, `{{...}}` title wrapping, TeX escaping, omit-empty) are identical to the Go version.

One Python-specific subtlety: `bib_escape` uses a single-pass dict lookup (`"".join(_ESCAPE_TABLE.get(c, c) for c in s)`) because chained `str.replace` calls would re-escape the `{` and `}` inside `\textbackslash{}`. Don't refactor it back to sequential replace.

## Tests

`test_app.py` uses pytest with `@pytest.mark.parametrize` for table-driven coverage (62 cases total). The Crossref fetcher is exercised via `unittest.mock.patch("urllib.request.urlopen", side_effect=...)` — no live HTTP, no local server thread. Coverage matches the Go test suite case-for-case:

- `test_normalize_doi` — every prefix variant, casing, whitespace
- `test_citation_text` — bioRxiv/medRxiv special case + every volume/pages combo
- `test_fetch_metadata_*` — happy path (asserts UA, URL), 404, article-number fallback, year priority fallthrough, bioRxiv institution fallback, trailing-slash on base
- `test_given_initials` — single, multi, smashed (`A.G.`), hyphenated, Unicode
- `test_bib_authors` — single + multi-author, multi-word surnames, multi-initial
- `test_bib_escape` — every escaped char (this is the regression that caught the sequential-replace bug during the Go → Python port)
- `test_bib_ascii_fold` — Müller, Émile, Zoë, naïve, Çelik
- `test_bib_key` — collision suffixes + diacritic input
