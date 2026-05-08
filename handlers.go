package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
)

func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	renderTemplate(w, http.StatusOK, "", getPapers())
}

func handleSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	cleanDOI := normalizeDOI(r.FormValue("doi"))
	if !doiRegex.MatchString(cleanDOI) {
		renderTemplate(w, http.StatusBadRequest, "Invalid DOI format. Please use 10.xxxx/xxxx or a DOI URL.", getPapers())
		return
	}

	exists, err := paperExists(cleanDOI)
	if err != nil {
		log.Println("Duplicate check error:", err)
		renderTemplate(w, http.StatusInternalServerError, "Database error. Please try again.", getPapers())
		return
	}
	if exists {
		renderTemplate(w, http.StatusConflict, "DOI is already in the list.", getPapers())
		return
	}

	paper, err := fetchMetadata(cleanDOI)
	if err != nil {
		log.Println("Metadata fetch error:", err)
		renderTemplate(w, http.StatusBadGateway, "Could not fetch metadata for that DOI. Please check it and try again.", getPapers())
		return
	}

	if err := insertPaper(paper); err != nil {
		log.Println("Insert error:", err)
		renderTemplate(w, http.StatusInternalServerError, "Failed to save paper. Please try again.", getPapers())
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err := deletePaper(r.FormValue("id")); err != nil {
		log.Println("Delete error:", err)
		renderTemplate(w, http.StatusInternalServerError, "Failed to delete paper.", getPapers())
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleExport(w http.ResponseWriter, r *http.Request) {
	papers := getPapers()

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="papers.md"`)

	for i, p := range papers {
		fmt.Fprintf(w, "### %d\n", i+1)
		fmt.Fprintf(w, "**%s**  \n", p.Title)
		if p.Year != "" {
			fmt.Fprintf(w, "%s (%s)  \n", p.Authors, p.Year)
		} else {
			fmt.Fprintf(w, "%s  \n", p.Authors)
		}
		fmt.Fprintf(w, "[%s](https://doi.org/%s)\n\n", citationText(p), p.DOI)
	}
}

func handleExportBib(w http.ResponseWriter, r *http.Request) {
	papers := getPapers()

	w.Header().Set("Content-Type", "application/x-bibtex; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="papers.bib"`)

	used := map[string]int{}
	for _, p := range papers {
		writeBibEntry(w, p, used)
	}
}

// citationText builds the display text for a paper's citation link.
// Preprint servers (bioRxiv, medRxiv) get "Journal:article_id" format;
// journals with volume/pages get "Journal Volume:Pages".
func citationText(p Paper) string {
	if p.Journal == "" {
		return p.DOI
	}
	switch strings.ToLower(p.Journal) {
	case "biorxiv", "medrxiv":
		if idx := strings.IndexByte(p.DOI, '/'); idx >= 0 {
			return p.Journal + ":" + p.DOI[idx+1:]
		}
	}
	if p.Volume != "" && p.Pages != "" {
		return p.Journal + " " + p.Volume + ":" + p.Pages
	}
	if p.Volume != "" {
		return p.Journal + " " + p.Volume
	}
	if p.Pages != "" {
		return p.Journal + " " + p.Pages
	}
	return p.Journal
}
