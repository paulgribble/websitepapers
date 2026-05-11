import base64
import io
import os
import secrets
import sqlite3

from flask import Flask, Response, redirect, render_template, request

from bibtex import write_bib_entry
from crossref import fetch_metadata
from db import Paper, delete_paper, get_papers, init_db, insert_paper, paper_exists
from doi import DOI_REGEX, normalize_doi

app = Flask(__name__)

BASIC_AUTH_USER = os.environ.get("BASIC_AUTH_USER", "")
BASIC_AUTH_PASS = os.environ.get("BASIC_AUTH_PASS", "")


def _basic_auth_ok(header: str) -> bool:
    if not header.startswith("Basic "):
        return False
    try:
        decoded = base64.b64decode(header[6:], validate=True).decode("utf-8")
    except (ValueError, UnicodeDecodeError):
        return False
    user, sep, pw = decoded.partition(":")
    if not sep:
        return False
    return (
        secrets.compare_digest(user, BASIC_AUTH_USER)
        and secrets.compare_digest(pw, BASIC_AUTH_PASS)
    )


@app.before_request
def _require_basic_auth():
    if not (BASIC_AUTH_USER and BASIC_AUTH_PASS):
        return
    # /up is the container HEALTHCHECK target; it must stay open.
    if request.path == "/up":
        return
    if _basic_auth_ok(request.headers.get("Authorization", "")):
        return
    return Response(
        "Authentication required",
        status=401,
        headers={"WWW-Authenticate": 'Basic realm="websitepapers"'},
    )


def render_page(status: int = 200, message: str = ""):
    return render_template("index.html", papers=get_papers(), message=message), status


def respond_err(status: int, msg: str, err: Exception):
    """Log the underlying error and render the page with a status code and
    user-facing message. The user-facing message doubles as the log label so
    operator-side log lines correlate with reported issues."""
    app.logger.error("%s: %s", msg, err)
    return render_page(status, msg)


@app.route("/", methods=["GET"])
def home():
    return render_page()


@app.route("/up", methods=["GET"])
def health():
    return "OK", 200, {"Content-Type": "text/plain; charset=utf-8"}


@app.route("/submit", methods=["GET", "POST"])
def submit():
    if request.method != "POST":
        return redirect("/", code=303)

    clean = normalize_doi(request.form.get("doi", ""))
    if not DOI_REGEX.match(clean):
        return render_page(400, "Invalid DOI format. Please use 10.xxxx/xxxx or a DOI URL.")

    try:
        if paper_exists(clean):
            return render_page(409, "DOI is already in the list.")
    except sqlite3.Error as e:
        return respond_err(500, "Database error. Please try again.", e)

    try:
        paper = fetch_metadata(clean)
    except Exception as e:
        return respond_err(502, "Could not fetch metadata for that DOI. Please check it and try again.", e)

    try:
        insert_paper(paper)
    except sqlite3.Error as e:
        return respond_err(500, "Failed to save paper. Please try again.", e)

    return redirect("/", code=303)


@app.route("/delete", methods=["GET", "POST"])
def delete():
    if request.method != "POST":
        return redirect("/", code=303)
    try:
        delete_paper(request.form.get("id", ""))
    except sqlite3.Error as e:
        return respond_err(500, "Failed to delete paper.", e)
    return redirect("/", code=303)


@app.route("/export", methods=["GET"])
def export_md():
    buf = io.StringIO()
    for i, p in enumerate(get_papers(), start=1):
        buf.write(f"### {i}\n")
        buf.write(f"**{p.title}**  \n")
        if p.year:
            buf.write(f"{p.authors} ({p.year})  \n")
        else:
            buf.write(f"{p.authors}  \n")
        buf.write(f"[{citation_text(p)}](https://doi.org/{p.doi})\n\n")
    return Response(
        buf.getvalue(),
        headers={
            "Content-Type": "text/markdown; charset=utf-8",
            "Content-Disposition": 'attachment; filename="papers.md"',
        },
    )


@app.route("/export.bib", methods=["GET"])
def export_bib():
    buf = io.StringIO()
    used: dict = {}
    for p in get_papers():
        write_bib_entry(buf, p, used)
    return Response(
        buf.getvalue(),
        headers={
            "Content-Type": "application/x-bibtex; charset=utf-8",
            "Content-Disposition": 'attachment; filename="papers.bib"',
        },
    )


def citation_text(p: Paper) -> str:
    """Display string for the markdown export's citation link.

    Preprint servers (bioRxiv, medRxiv) get "Journal:article_id" format;
    journals with volume/pages get "Journal Volume:Pages".
    """
    if not p.journal:
        return p.doi
    if p.journal.lower() in ("biorxiv", "medrxiv"):
        idx = p.doi.find("/")
        if idx >= 0:
            return f"{p.journal}:{p.doi[idx+1:]}"
    if p.volume and p.pages:
        return f"{p.journal} {p.volume}:{p.pages}"
    if p.volume:
        return f"{p.journal} {p.volume}"
    if p.pages:
        return f"{p.journal} {p.pages}"
    return p.journal


if __name__ == "__main__":
    init_db()
    print("Server starting at http://localhost:8080")
    app.run(host="127.0.0.1", port=8080, debug=False)
