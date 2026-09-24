import json
import urllib.error
import urllib.parse
import urllib.request

from db import Paper

CROSSREF_BASE = "https://api.crossref.org"
DATACITE_BASE = "https://api.datacite.org"
USER_AGENT = "websitepapers/0.1 (mailto:pgribblle@uwo.ca)"
TIMEOUT = 10  # seconds


class DOINotFound(RuntimeError):
    """Raised when neither Crossref nor DataCite knows the DOI."""


def _get_json(url: str) -> dict:
    """GET a JSON document, identifying the client via User-Agent (Crossref polite pool)."""
    req = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
        return json.load(resp)


def fetch_metadata(doi: str) -> Paper:
    """Look up a DOI and return a Paper.

    Tries Crossref first (journals, bioRxiv, medRxiv). On a Crossref 404 the DOI
    is probably registered with DataCite instead (arXiv, Zenodo, Figshare, OSF,
    datasets), so DataCite is queried next. Raises DOINotFound if both return 404
    and RuntimeError for any other upstream HTTP failure.
    """
    quoted = urllib.parse.quote(doi, safe="")
    try:
        body = _get_json(f"{CROSSREF_BASE.rstrip('/')}/works/{quoted}")
        return _from_crossref(doi, body)
    except urllib.error.HTTPError as e:
        if e.code != 404:
            raise RuntimeError(f"Crossref error (status {e.code})") from e

    try:
        body = _get_json(f"{DATACITE_BASE.rstrip('/')}/dois/{quoted}")
        return _from_datacite(doi, body)
    except urllib.error.HTTPError as e:
        if e.code == 404:
            raise DOINotFound("DOI not found in Crossref or DataCite") from e
        raise RuntimeError(f"DataCite error (status {e.code})") from e


def _from_crossref(doi: str, body: dict) -> Paper:
    m = body.get("message") or {}

    title = (m.get("title") or [""])[0]
    journal = (m.get("container-title") or [""])[0]
    if not journal:
        inst = m.get("institution") or []
        journal = inst[0].get("name", "") if inst else ""
    volume = m.get("volume") or ""
    pages = m.get("page") or ""
    if not volume and not pages:
        pages = m.get("article-number") or ""

    authors = []
    for a in m.get("author") or []:
        name = a.get("family", "")
        initials = given_initials(a.get("given", ""))
        if initials:
            name = f"{name} {initials}" if name else initials
        if name:
            authors.append(name)

    year = ""
    for key in ("published-print", "published-online", "issued"):
        dp = (m.get(key) or {}).get("date-parts") or []
        if dp and dp[0]:
            year = str(dp[0][0])
            break

    return Paper(
        doi=doi,
        title=title,
        authors=", ".join(authors),
        journal=journal,
        year=year,
        volume=volume,
        pages=pages,
    )


def _from_datacite(doi: str, body: dict) -> Paper:
    """Map a DataCite REST API (JSON:API) document onto a Paper.

    DataCite has no journal concept; ``container.title`` is used when present,
    otherwise the publisher (``"arXiv"``, ``"Zenodo"``, ...). Newer API versions
    return ``publisher`` as an object with a ``name`` key; older ones as a string.
    """
    a = (body.get("data") or {}).get("attributes") or {}

    titles = a.get("titles") or []
    title = (titles[0].get("title") if titles else "") or ""

    container = a.get("container") or {}
    journal = container.get("title") or ""
    if not journal:
        pub = a.get("publisher") or ""
        journal = pub.get("name", "") if isinstance(pub, dict) else pub

    volume = container.get("volume") or ""
    first, last = container.get("firstPage") or "", container.get("lastPage") or ""
    pages = f"{first}-{last}" if first and last else (first or last)

    authors = []
    for c in a.get("creators") or []:
        family = c.get("familyName") or ""
        given = c.get("givenName") or ""
        if not family and not given:
            # Organisations, or personal names only given as "Family, Given"
            raw = c.get("name") or ""
            family, _, given = raw.partition(",")
            family, given = family.strip(), given.strip()
        name = family
        initials = given_initials(given)
        if initials:
            name = f"{name} {initials}" if name else initials
        if name:
            authors.append(name)

    year = a.get("publicationYear")
    year = str(year) if year else ""

    return Paper(
        doi=doi,
        title=title,
        authors=", ".join(authors),
        journal=journal,
        year=year,
        volume=volume,
        pages=pages,
    )


def given_initials(given: str) -> str:
    """Emit one initial per letter-run.

    Splits on any non-letter so smashed initials work: "Andrew A.G." yields
    "A. A. G." rather than "A. A." Handles hyphenated names
    ("Marie-Claude" -> "M. C.") and Unicode ("Émile" -> "É.").
    """
    parts = []
    run = ""
    for ch in given:
        if ch.isalpha():
            if not run:
                run = ch
        else:
            if run:
                parts.append(run + ".")
                run = ""
    if run:
        parts.append(run + ".")
    return " ".join(parts)
