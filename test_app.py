import json
import urllib.error
from unittest.mock import MagicMock, patch

import pytest

import crossref
from app import citation_text
from bibtex import bib_ascii_fold, bib_authors, bib_escape, bib_key
from crossref import fetch_metadata, given_initials
from db import Paper
from doi import normalize_doi


# ---------- Crossref mock plumbing ----------

def _mock_response(body):
    if isinstance(body, (dict, list)):
        body = json.dumps(body)
    if isinstance(body, str):
        body = body.encode()
    resp = MagicMock()
    resp.read.return_value = body
    resp.__enter__ = MagicMock(return_value=resp)
    resp.__exit__ = MagicMock(return_value=None)
    return resp


def _fake_urlopen(body, captured=None):
    def fake(req, timeout=None):
        if captured is not None:
            captured["url"] = req.full_url
            captured["user_agent"] = req.get_header("User-agent")
        return _mock_response(body)
    return fake


# ---------- normalize_doi ----------

@pytest.mark.parametrize("inp,want", [
    ("", ""),
    ("10.1038/abc", "10.1038/abc"),
    ("10.1038/ABC", "10.1038/abc"),
    ("  10.1038/abc  ", "10.1038/abc"),
    ("https://doi.org/10.1038/abc", "10.1038/abc"),
    ("http://doi.org/10.1038/abc", "10.1038/abc"),
    ("https://dx.doi.org/10.1038/abc", "10.1038/abc"),
    ("http://dx.doi.org/10.1038/abc", "10.1038/abc"),
    ("dx.doi.org/10.1038/abc", "10.1038/abc"),
    ("doi.org/10.1038/abc", "10.1038/abc"),
    ("HTTPS://DOI.ORG/10.1038/ABC", "10.1038/abc"),
    ("Https://Doi.Org/10.1038/AbC", "10.1038/abc"),
])
def test_normalize_doi(inp, want):
    assert normalize_doi(inp) == want


# ---------- citation_text ----------

@pytest.mark.parametrize("p,want", [
    (Paper(doi="10.1/x"), "10.1/x"),
    (Paper(doi="10.1101/2026.04.27.721195", journal="bioRxiv"), "bioRxiv:2026.04.27.721195"),
    (Paper(doi="10.1101/2026.04.27.721195", journal="BioRxiv"), "BioRxiv:2026.04.27.721195"),
    (Paper(doi="10.1101/abc", journal="medRxiv"), "medRxiv:abc"),
    (Paper(journal="J Neurophysiol", volume="135", pages="1175-1185"), "J Neurophysiol 135:1175-1185"),
    (Paper(journal="Nature", volume="600"), "Nature 600"),
    (Paper(journal="Nature", pages="12"), "Nature 12"),
    (Paper(journal="Nature"), "Nature"),
])
def test_citation_text(p, want):
    assert citation_text(p) == want


# ---------- fetch_metadata ----------

def test_fetch_metadata_happy_path():
    body = {"message": {
        "title": ["Demo Title"],
        "container-title": ["J Demo"],
        "volume": "42",
        "page": "1-10",
        "author": [
            {"given": "Alice", "family": "Smith"},
            {"given": "Paul L", "family": "Gribble"},
            {"given": "Émile", "family": "Zola"},
        ],
        "published-print": {"date-parts": [[2024, 5, 1]]},
    }}
    captured = {}
    with patch("urllib.request.urlopen", side_effect=_fake_urlopen(body, captured)):
        p = fetch_metadata("10.1/x")
    assert "/works/" in captured["url"]
    assert "websitepapers" in captured["user_agent"]
    assert p.title == "Demo Title"
    assert p.journal == "J Demo"
    assert p.volume == "42"
    assert p.pages == "1-10"
    assert p.year == "2024"
    assert p.authors == "Smith A., Gribble P. L., Zola É."


def test_fetch_metadata_404():
    def fake(req, timeout=None):
        raise urllib.error.HTTPError(req.full_url, 404, "nope", {}, None)
    with patch("urllib.request.urlopen", side_effect=fake):
        with pytest.raises(RuntimeError):
            fetch_metadata("10.1/x")


def test_fetch_metadata_article_number_fallback():
    body = {"message": {
        "title": ["T"],
        "container-title": ["J"],
        "article-number": "e12345",
        "issued": {"date-parts": [[2025]]},
    }}
    with patch("urllib.request.urlopen", side_effect=_fake_urlopen(body)):
        p = fetch_metadata("10.1/x")
    assert p.pages == "e12345"
    assert p.year == "2025"


def test_fetch_metadata_biorxiv_institution_fallback():
    body = {"message": {
        "title": ["Preprint Title"],
        "container-title": [],
        "institution": [{"name": "bioRxiv"}],
        "type": "posted-content",
        "subtype": "preprint",
        "issued": {"date-parts": [[2020]]},
    }}
    with patch("urllib.request.urlopen", side_effect=_fake_urlopen(body)):
        p = fetch_metadata("10.1101/2020.03.25.008466")
    assert p.journal == "bioRxiv"


def test_fetch_metadata_year_falls_through_to_published_online():
    body = {"message": {
        "title": ["T"],
        "container-title": ["J"],
        "published-online": {"date-parts": [[2023, 2, 3]]},
        "issued": {"date-parts": [[2022]]},
    }}
    with patch("urllib.request.urlopen", side_effect=_fake_urlopen(body)):
        p = fetch_metadata("10.1/x")
    assert p.year == "2023"


def test_fetch_metadata_trailing_slash_on_base():
    body = {"message": {"title": ["T"]}}
    orig = crossref.CROSSREF_BASE
    captured = {}
    try:
        crossref.CROSSREF_BASE = orig + "/"
        with patch("urllib.request.urlopen", side_effect=_fake_urlopen(body, captured)):
            p = fetch_metadata("10.1/x")
        assert p.title == "T"
        scheme, _, rest = captured["url"].partition("://")
        assert "//works/" not in rest
    finally:
        crossref.CROSSREF_BASE = orig


# ---------- given_initials ----------

@pytest.mark.parametrize("inp,want", [
    ("", ""),
    ("Paul", "P."),
    ("Paul L", "P. L."),
    ("Paul L.", "P. L."),
    ("Paul Luc", "P. L."),
    ("  Paul   L  ", "P. L."),
    ("Émile", "É."),
    ("J", "J."),
    ("Andrew A.G.", "A. A. G."),
    ("A.G.", "A. G."),
    ("A. G.", "A. G."),
    ("Marie-Claude", "M. C."),
])
def test_given_initials(inp, want):
    assert given_initials(inp) == want


# ---------- bib_authors ----------

@pytest.mark.parametrize("inp,want", [
    ("", ""),
    ("Smith", "Smith"),
    ("Smith J.", "Smith, J."),
    ("Smith J., Jones A.", "Smith, J. and Jones, A."),
    ("Smith J., Jones A., Doe X.", "Smith, J. and Jones, A. and Doe, X."),
    ("Gribble P. L.", "Gribble, P. L."),
    ("van der Berg J.", "van der Berg, J."),
    ("van der Berg P. L.", "van der Berg, P. L."),
    ("Zola É.", "Zola, É."),
    ("Smith J., Gribble P. L.", "Smith, J. and Gribble, P. L."),
])
def test_bib_authors(inp, want):
    assert bib_authors(inp) == want


# ---------- bib_escape ----------

@pytest.mark.parametrize("inp,want", [
    ("", ""),
    ("plain text", "plain text"),
    ("50% off & free", r"50\% off \& free"),
    ("a_b#c$d", r"a\_b\#c\$d"),
    ("{x}", r"\{x\}"),
    ("a\\b", r"a\textbackslash{}b"),
])
def test_bib_escape(inp, want):
    assert bib_escape(inp) == want


# ---------- bib_ascii_fold ----------

@pytest.mark.parametrize("inp,want", [
    ("", ""),
    ("ASCII", "ASCII"),
    ("Müller", "Muller"),
    ("Émile", "Emile"),
    ("Zoë", "Zoe"),
    ("naïve", "naive"),
    ("Çelik", "Celik"),
])
def test_bib_ascii_fold(inp, want):
    assert bib_ascii_fold(inp) == want


# ---------- bib_key ----------

def test_bib_key():
    used: dict = {}
    p1 = Paper(authors="Smith J., Jones A.", year="2024", title="A Cool Paper About Stuff")
    assert bib_key(p1, used) == "smith2024a"
    assert bib_key(p1, used) == "smith2024a_2"

    p2 = Paper()
    assert bib_key(p2, used) == "paper"

    p3 = Paper(authors="Müller H.", year="2024", title="On X")
    assert bib_key(p3, used) == "muller2024on"
