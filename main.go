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

var crossrefBase = "https://api.crossref.org"

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

	if _, err := db.Exec("UPDATE papers SET doi = LOWER(doi) WHERE doi != LOWER(doi)"); err != nil {
		log.Println("DOI lowercasing migration:", err)
	}

	http.HandleFunc("/", handleHome)
	http.HandleFunc("/submit", handleSubmit)
	http.HandleFunc("/delete", handleDelete)
	http.HandleFunc("/export", handleExport)
	http.HandleFunc("/export.bib", handleExportBib)

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

func handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if _, err := db.Exec("DELETE FROM papers WHERE id=?", r.FormValue("id")); err != nil {
		log.Println("Delete error:", err)
		renderTemplate(w, "Failed to delete paper.", getPapers())
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleExportBib(w http.ResponseWriter, r *http.Request) {
	papers := getPapers()

	w.Header().Set("Content-Type", "application/x-bibtex; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="papers.bib"`)

	used := map[string]int{}
	for _, p := range papers {
		key := bibKey(p, used)
		fmt.Fprintf(w, "@article{%s,\n", key)
		if p.Authors != "" {
			fmt.Fprintf(w, "  author  = {%s},\n", bibAuthors(p.Authors))
		}
		if p.Title != "" {
			fmt.Fprintf(w, "  title   = {{%s}},\n", bibEscape(p.Title))
		}
		if p.Journal != "" {
			fmt.Fprintf(w, "  journal = {%s},\n", bibEscape(p.Journal))
		}
		if p.Year != "" {
			fmt.Fprintf(w, "  year    = {%s},\n", p.Year)
		}
		if p.Volume != "" {
			fmt.Fprintf(w, "  volume  = {%s},\n", bibEscape(p.Volume))
		}
		if p.Pages != "" {
			fmt.Fprintf(w, "  pages   = {%s},\n", bibEscape(p.Pages))
		}
		fmt.Fprintf(w, "  doi     = {%s}\n", p.DOI)
		fmt.Fprint(w, "}\n\n")
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

func bibKey(p Paper, used map[string]int) string {
	var first string
	if p.Authors != "" {
		if i := strings.IndexAny(p.Authors, " ,"); i > 0 {
			first = p.Authors[:i]
		} else {
			first = p.Authors
		}
	}
	var titleWord string
	for _, w := range strings.Fields(p.Title) {
		if w := bibAlpha(w); w != "" {
			titleWord = w
			break
		}
	}
	base := strings.ToLower(bibAlpha(first) + p.Year + titleWord)
	if base == "" {
		base = "paper"
	}
	used[base]++
	if used[base] == 1 {
		return base
	}
	return fmt.Sprintf("%s_%d", base, used[base])
}

func givenInitials(given string) string {
	var parts []string
	for _, tok := range strings.Fields(given) {
		for _, r := range tok {
			parts = append(parts, string(r)+".")
			break
		}
	}
	return strings.Join(parts, " ")
}

func bibAlpha(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func bibAuthors(s string) string {
	parts := strings.Split(s, ", ")
	for i, p := range parts {
		tokens := strings.Fields(p)
		split := -1
		for j, t := range tokens {
			if isInitial(t) {
				split = j
				break
			}
		}
		if split > 0 {
			parts[i] = strings.Join(tokens[:split], " ") + ", " + strings.Join(tokens[split:], " ")
		}
	}
	return strings.Join(parts, " and ")
}

func isInitial(t string) bool {
	r := []rune(t)
	return len(r) == 2 && r[1] == '.'
}

func bibEscape(s string) string {
	r := strings.NewReplacer(
		`\`, `\textbackslash{}`,
		`&`, `\&`,
		`%`, `\%`,
		`_`, `\_`,
		`#`, `\#`,
		`$`, `\$`,
		`{`, `\{`,
		`}`, `\}`,
	)
	return r.Replace(s)
}

func fetchMetadata(doi string) (Paper, error) {
	req, err := http.NewRequest("GET", crossrefBase+"/works/"+url.PathEscape(doi), nil)
	if err != nil {
		return Paper{DOI: doi}, err
	}
	req.Header.Set("User-Agent", "websitepapers/0.1 (mailto:pgribble@uwo.ca)")

	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
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
			Institution    []struct {
				Name string `json:"name"`
			} `json:"institution"`
			Volume        string `json:"volume"`
			Page          string `json:"page"`
			ArticleNumber string `json:"article-number"`
			Author        []struct {
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
	} else if len(m.Institution) > 0 {
		paper.Journal = m.Institution[0].Name
	}
	paper.Volume = m.Volume
	paper.Pages = m.Page
	if paper.Volume == "" && paper.Pages == "" && m.ArticleNumber != "" {
		paper.Pages = m.ArticleNumber
	}

	var authors []string
	for _, a := range m.Author {
		name := a.Family
		if initials := givenInitials(a.Given); initials != "" {
			name += " " + initials
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

func normalizeDOI(input string) string {
	input = strings.TrimSpace(input)
	lower := strings.ToLower(input)
	for _, p := range []string{
		"https://doi.org/", "http://doi.org/",
		"https://dx.doi.org/", "http://dx.doi.org/",
		"dx.doi.org/", "doi.org/",
	} {
		if strings.HasPrefix(lower, p) {
			return lower[len(p):]
		}
	}
	return lower
}

func renderTemplate(w http.ResponseWriter, msg string, list []Paper) {
	if err := tmpl.Execute(w, PageData{Papers: list, Message: msg}); err != nil {
		log.Println("Template error:", err)
	}
}
