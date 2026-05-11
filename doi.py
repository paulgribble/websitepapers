import re

DOI_REGEX = re.compile(r"^10\.\d{4,}(?:\.\d+)?/\S+$", re.IGNORECASE)

_URL_PREFIXES = (
    "https://doi.org/",
    "http://doi.org/",
    "https://dx.doi.org/",
    "http://dx.doi.org/",
    "dx.doi.org/",
    "doi.org/",
)


def normalize_doi(s: str) -> str:
    """Trim, lowercase, and strip known doi.org URL prefixes.

    DOIs are officially case-insensitive, so storage and comparison
    use the lowercase form.
    """
    s = s.strip().lower()
    for prefix in _URL_PREFIXES:
        if s.startswith(prefix):
            return s[len(prefix):]
    return s
