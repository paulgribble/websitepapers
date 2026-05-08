package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

var crossrefBase = "https://api.crossref.org"

type crossrefResponse struct {
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

// fetchMetadata calls the Crossref API for a single DOI and returns a Paper
// populated with the fields we display. Identifies the client via User-Agent
// for Crossref's "polite pool".
func fetchMetadata(doi string) (Paper, error) {
	base := strings.TrimRight(crossrefBase, "/")
	req, err := http.NewRequest("GET", base+"/works/"+url.PathEscape(doi), nil)
	if err != nil {
		return Paper{}, err
	}
	req.Header.Set("User-Agent", "websitepapers/0.1 (mailto:pgribble@uwo.ca)")

	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return Paper{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Paper{}, fmt.Errorf("DOI not found (status %d)", resp.StatusCode)
	}

	var result crossrefResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return Paper{}, err
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

// givenInitials emits one initial per letter-run in the given name. Splits
// on any non-letter so smashed initials work too: "Andrew A.G." yields
// "A. A. G." rather than "A. A." (which whitespace-only splitting produced).
func givenInitials(given string) string {
	var parts []string
	for _, tok := range strings.FieldsFunc(given, func(r rune) bool { return !unicode.IsLetter(r) }) {
		for _, r := range tok {
			parts = append(parts, string(r)+".")
			break
		}
	}
	return strings.Join(parts, " ")
}
