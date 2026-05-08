package main

import (
	"database/sql"
	"log"
	"strings"
)

// openDB opens the SQLite file, creates the schema if needed, and runs the
// idempotent migrations.
func openDB(path string) (*sql.DB, error) {
	d, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	if _, err := d.Exec(`CREATE TABLE IF NOT EXISTS papers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		doi TEXT UNIQUE,
		title TEXT,
		authors TEXT,
		journal TEXT,
		pub_date TEXT
	);`); err != nil {
		return nil, err
	}
	runMigration(d, "add volume column", "ALTER TABLE papers ADD COLUMN volume TEXT")
	runMigration(d, "add page column", "ALTER TABLE papers ADD COLUMN page TEXT")
	runMigration(d, "lowercase DOIs", "UPDATE papers SET doi = LOWER(doi) WHERE doi != LOWER(doi)")
	return d, nil
}

// runMigration executes one migration. SQLite returns "duplicate column name"
// for idempotent ADD COLUMN on every restart after the first; that one we
// silently ignore. Anything else is logged.
func runMigration(d *sql.DB, name, sql string) {
	if _, err := d.Exec(sql); err != nil {
		if strings.Contains(err.Error(), "duplicate column name") {
			return
		}
		log.Printf("migration %q: %v", name, err)
	}
}

func getPapers() []Paper {
	rows, err := db.Query("SELECT id, doi, title, authors, journal, pub_date, COALESCE(volume,''), COALESCE(page,'') FROM papers ORDER BY id DESC")
	if err != nil {
		log.Println("Query error:", err)
		return nil
	}
	defer rows.Close()

	var list []Paper
	for rows.Next() {
		var p Paper
		if err := rows.Scan(&p.ID, &p.DOI, &p.Title, &p.Authors, &p.Journal, &p.Year, &p.Volume, &p.Pages); err != nil {
			log.Println("Scan error:", err)
			continue
		}
		list = append(list, p)
	}
	if err := rows.Err(); err != nil {
		log.Println("Rows error:", err)
	}
	return list
}

func paperExists(doi string) (bool, error) {
	var exists bool
	err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM papers WHERE doi=?)", doi).Scan(&exists)
	return exists, err
}

func insertPaper(p Paper) error {
	_, err := db.Exec(
		"INSERT INTO papers (doi, title, authors, journal, pub_date, volume, page) VALUES (?, ?, ?, ?, ?, ?, ?)",
		p.DOI, p.Title, p.Authors, p.Journal, p.Year, p.Volume, p.Pages,
	)
	return err
}

func deletePaper(id string) error {
	_, err := db.Exec("DELETE FROM papers WHERE id=?", id)
	return err
}
