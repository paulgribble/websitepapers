package main

import (
	"database/sql"
	"html/template"
	"log"
	"net/http"

	_ "github.com/mattn/go-sqlite3"
)

type Paper struct {
	ID      int
	DOI     string
	Title   string
	Authors string
	Journal string
	Year    string
	Volume  string
	Pages   string
}

type PageData struct {
	Papers  []Paper
	Message string
}

var (
	db   *sql.DB
	tmpl *template.Template
)

func main() {
	var err error

	tmpl, err = template.ParseFiles("index.html")
	if err != nil {
		log.Fatal(err)
	}

	db, err = openDB("./dois.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	http.HandleFunc("/", handleHome)
	http.HandleFunc("/submit", handleSubmit)
	http.HandleFunc("/delete", handleDelete)
	http.HandleFunc("/export", handleExport)
	http.HandleFunc("/export.bib", handleExportBib)

	log.Println("Server starting at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func renderTemplate(w http.ResponseWriter, status int, msg string, list []Paper) {
	w.WriteHeader(status)
	if err := tmpl.Execute(w, PageData{Papers: list, Message: msg}); err != nil {
		log.Println("Template error:", err)
	}
}
