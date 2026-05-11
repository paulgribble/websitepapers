import unicodedata
from typing import TextIO

from db import Paper


_ESCAPE_TABLE = {
    "\\": r"\textbackslash{}",
    "&": r"\&",
    "%": r"\%",
    "_": r"\_",
    "#": r"\#",
    "$": r"\$",
    "{": r"\{",
    "}": r"\}",
}


def write_bib_entry(out: TextIO, p: Paper, used: dict) -> None:
    """Write one @article{...} block. `used` tracks emitted citation keys
    so collisions within a single export get _2, _3 suffixes."""
    key = bib_key(p, used)
    out.write(f"@article{{{key},\n")
    if p.authors:
        out.write(f"  author  = {{{bib_authors(p.authors)}}},\n")
    if p.title:
        out.write(f"  title   = {{{{{bib_escape(p.title)}}}}},\n")
    if p.journal:
        out.write(f"  journal = {{{bib_escape(p.journal)}}},\n")
    if p.year:
        out.write(f"  year    = {{{p.year}}},\n")
    if p.volume:
        out.write(f"  volume  = {{{bib_escape(p.volume)}}},\n")
    if p.pages:
        out.write(f"  pages   = {{{bib_escape(p.pages)}}},\n")
    out.write(f"  doi     = {{{p.doi}}}\n}}\n\n")


def bib_key(p: Paper, used: dict) -> str:
    """firstAuthorSurname + year + firstTitleWord, lowercased, ASCII-folded.
    Collisions get _2, _3 suffixes via `used`."""
    first = p.authors
    if p.authors:
        candidates = [i for i in (p.authors.find(" "), p.authors.find(",")) if i > 0]
        if candidates:
            first = p.authors[:min(candidates)]

    title_word = ""
    for word in p.title.split():
        alpha = bib_alpha(word)
        if alpha:
            title_word = alpha
            break

    base = (bib_alpha(first) + p.year + title_word).lower()
    if not base:
        base = "paper"

    used[base] = used.get(base, 0) + 1
    return base if used[base] == 1 else f"{base}_{used[base]}"


def bib_alpha(s: str) -> str:
    """Keep ASCII letters/digits after ASCII-folding."""
    return "".join(c for c in bib_ascii_fold(s) if c.isascii() and c.isalnum())


def bib_ascii_fold(s: str) -> str:
    """NFD-normalize and strip combining marks: 'Müller' -> 'Muller'.

    Used for citation key generation only; displayed author text is
    unaffected.
    """
    return "".join(
        c for c in unicodedata.normalize("NFD", s)
        if unicodedata.category(c) != "Mn"
    )


def bib_authors(s: str) -> str:
    """Convert stored "Family I., Family I." to BibTeX
    "Family, I. and Family, I." The split point is the first
    whitespace-separated token shaped like an initial (<rune>.) so
    multi-word surnames ("van der Berg") stay intact.
    """
    out = []
    for part in s.split(", "):
        tokens = part.split()
        split = -1
        for j, t in enumerate(tokens):
            if _is_initial(t):
                split = j
                break
        if split > 0:
            out.append(" ".join(tokens[:split]) + ", " + " ".join(tokens[split:]))
        else:
            out.append(part)
    return " and ".join(out)


def _is_initial(t: str) -> bool:
    return len(t) == 2 and t[1] == "."


def bib_escape(s: str) -> str:
    """Escape \\ & % _ # $ { } for TeX. Single-pass so braces inside a
    `\\textbackslash{}` replacement aren't themselves re-escaped."""
    return "".join(_ESCAPE_TABLE.get(c, c) for c in s)
