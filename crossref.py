import json
import urllib.error
import urllib.parse
import urllib.request

from db import Paper

CROSSREF_BASE = "https://api.crossref.org"
USER_AGENT = "websitepapers/0.1 (mailto:pgribblle@uwo.ca)"
TIMEOUT = 10  # seconds


def fetch_metadata(doi: str) -> Paper:
    """Call the Crossref API for a single DOI and return a Paper.

    Identifies the client via User-Agent for Crossref's "polite pool".
    """
    base = CROSSREF_BASE.rstrip("/")
    url = f"{base}/works/{urllib.parse.quote(doi, safe='')}"
    req = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
            body = json.load(resp)
    except urllib.error.HTTPError as e:
        raise RuntimeError(f"DOI not found (status {e.code})") from e

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
