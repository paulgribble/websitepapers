package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var doiRegex = regexp.MustCompile(`(?i)^10\.\d{4,}(?:\.\d+)?/\S+$`)

type Paper struct {
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

var db *sql.DB
var tmpl *template.Template

func main() {
	var err error

	tmpl, err = template.ParseFiles("index.html")
	if err != nil {
		log.Fatal(err)
	}

	db, err = sql.Open("sqlite3", "./dois.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS papers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		doi TEXT UNIQUE,
		title TEXT,
		authors TEXT,
		journal TEXT,
		pub_date TEXT
	);`)
	if err != nil {
		log.Fatal(err)
	}

	// Migrate: add volume/page columns for existing databases
	db.Exec("ALTER TABLE papers ADD COLUMN volume TEXT")
	db.Exec("ALTER TABLE papers ADD COLUMN page TEXT")

	http.HandleFunc("/", handleHome)
	http.HandleFunc("/submit", handleSubmit)
	http.HandleFunc("/export", handleExport)

	log.Println("Server starting at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// --- HANDLERS ---

func handleHome(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "", getPapers())
}

func handleSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	rawInput := r.FormValue("doi")
	cleanDOI := normalizeDOI(rawInput)

	if !doiRegex.MatchString(cleanDOI) {
		renderTemplate(w, "Invalid DOI format. Please use 10.xxxx/xxxx or a DOI URL.", getPapers())
		return
	}

	var exists bool
	if err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM papers WHERE doi=?)", cleanDOI).Scan(&exists); err != nil {
		log.Println("Duplicate check error:", err)
		renderTemplate(w, "Database error. Please try again.", getPapers())
		return
	}
	if exists {
		renderTemplate(w, "DOI is already in the list.", getPapers())
		return
	}

	paper, err := fetchMetadata(cleanDOI)
	if err != nil {
		log.Println("Metadata fetch error:", err)
		renderTemplate(w, "Could not fetch metadata for that DOI. Please check it and try again.", getPapers())
		return
	}

	_, err = db.Exec(
		"INSERT INTO papers (doi, title, authors, journal, pub_date, volume, page) VALUES (?, ?, ?, ?, ?, ?, ?)",
		paper.DOI, paper.Title, paper.Authors, paper.Journal, paper.Year, paper.Volume, paper.Pages,
	)
	if err != nil {
		log.Println("Insert error:", err)
		renderTemplate(w, "Failed to save paper. Please try again.", getPapers())
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

// --- HELPERS ---

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

func fetchMetadata(doi string) (Paper, error) {
	apiURL := &url.URL{
		Scheme: "https",
		Host:   "api.crossref.org",
		Path:   "/works/" + doi,
	}
	client := http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(apiURL.String())
	if err != nil {
		return Paper{DOI: doi}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Paper{DOI: doi}, fmt.Errorf("DOI not found (status %d)", resp.StatusCode)
	}

	var result struct {
		Message struct {
			Title          []string `json:"title"`
			ContainerTitle []string `json:"container-title"`
			Volume         string   `json:"volume"`
			Page           string   `json:"page"`
			ArticleNumber  string   `json:"article-number"`
			Author         []struct {
				Given  string `json:"given"`
				Family string `json:"family"`
			} `json:"author"`
			PublishedPrint struct {
				DateParts [][]int `json:"date-parts"`
			} `json:"published-print"`
			PublishedOnline struct {
				DateParts [][]int `json:"date-parts"`
			} `json:"published-online"`
			Issued struct {
				DateParts [][]int `json:"date-parts"`
			} `json:"issued"`
		} `json:"message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return Paper{DOI: doi}, err
	}

	m := result.Message
	paper := Paper{DOI: doi}

	if len(m.Title) > 0 {
		paper.Title = m.Title[0]
	}
	if len(m.ContainerTitle) > 0 {
		paper.Journal = m.ContainerTitle[0]
	}
	paper.Volume = m.Volume
	paper.Pages = m.Page
	if paper.Volume == "" && paper.Pages == "" && m.ArticleNumber != "" {
		paper.Pages = m.ArticleNumber
	}

	var authors []string
	for _, a := range m.Author {
		name := a.Family
		if len(a.Given) > 0 {
			name += " " + string(a.Given[0]) + "."
		}
		authors = append(authors, name)
	}
	paper.Authors = strings.Join(authors, ", ")

	for _, dp := range [][][]int{m.PublishedPrint.DateParts, m.PublishedOnline.DateParts, m.Issued.DateParts} {
		if len(dp) > 0 && len(dp[0]) > 0 {
			paper.Year = fmt.Sprintf("%d", dp[0][0])
			break
		}
	}

	return paper, nil
}

func getPapers() []Paper {
	rows, err := db.Query("SELECT doi, title, authors, journal, pub_date, COALESCE(volume,''), COALESCE(page,'') FROM papers ORDER BY id DESC")
	if err != nil {
		log.Println("Query error:", err)
		return nil
	}
	defer rows.Close()

	var list []Paper
	for rows.Next() {
		var p Paper
		if err := rows.Scan(&p.DOI, &p.Title, &p.Authors, &p.Journal, &p.Year, &p.Volume, &p.Pages); err != nil {
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

func normalizeDOI(input string) string {
	input = strings.TrimSpace(input)
	lower := strings.ToLower(input)
	for _, p := range []string{
		"https://doi.org/", "http://doi.org/",
		"https://dx.doi.org/", "http://dx.doi.org/",
		"dx.doi.org/", "doi.org/",
	} {
		if strings.HasPrefix(lower, p) {
			return input[len(p):]
		}
	}
	return input
}

func renderTemplate(w http.ResponseWriter, msg string, list []Paper) {
	if err := tmpl.Execute(w, PageData{Papers: list, Message: msg}); err != nil {
		log.Println("Template error:", err)
	}
}
