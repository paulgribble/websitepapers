import os
import sqlite3
from dataclasses import dataclass

DB_PATH = os.environ.get("DB_PATH", "./dois.db")

_SCHEMA = """
CREATE TABLE IF NOT EXISTS papers (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    doi      TEXT UNIQUE,
    title    TEXT,
    authors  TEXT,
    journal  TEXT,
    pub_date TEXT
)
"""

_MIGRATIONS = [
    "ALTER TABLE papers ADD COLUMN volume TEXT",
    "ALTER TABLE papers ADD COLUMN page TEXT",
    "UPDATE papers SET doi = LOWER(doi) WHERE doi != LOWER(doi)",
]


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


def _connect():
    return sqlite3.connect(DB_PATH)


def init_db() -> None:
    """Create the papers table and run idempotent migrations."""
    with _connect() as conn:
        conn.execute(_SCHEMA)
        for sql in _MIGRATIONS:
            try:
                conn.execute(sql)
            except sqlite3.OperationalError as e:
                # ALTER TABLE re-runs on every startup; swallow only that one.
                if "duplicate column name" not in str(e):
                    raise
        conn.commit()


def get_papers() -> list[Paper]:
    with _connect() as conn:
        rows = conn.execute(
            "SELECT id, doi, title, authors, journal, pub_date, "
            "COALESCE(volume,''), COALESCE(page,'') "
            "FROM papers ORDER BY id DESC"
        ).fetchall()
    return [Paper(*row) for row in rows]


def paper_exists(doi: str) -> bool:
    with _connect() as conn:
        (exists,) = conn.execute(
            "SELECT EXISTS(SELECT 1 FROM papers WHERE doi=?)", (doi,)
        ).fetchone()
    return bool(exists)


def insert_paper(p: Paper) -> None:
    with _connect() as conn:
        conn.execute(
            "INSERT INTO papers (doi, title, authors, journal, pub_date, volume, page) "
            "VALUES (?, ?, ?, ?, ?, ?, ?)",
            (p.doi, p.title, p.authors, p.journal, p.year, p.volume, p.pages),
        )
        conn.commit()


def delete_paper(paper_id: str) -> None:
    with _connect() as conn:
        conn.execute("DELETE FROM papers WHERE id=?", (paper_id,))
        conn.commit()


def delete_all_papers() -> None:
    with _connect() as conn:
        conn.execute("DELETE FROM papers")
        conn.commit()
