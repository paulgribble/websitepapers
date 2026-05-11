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

// pick walks a decoded JSON value using string keys for objects and int
// indices for arrays. Returns nil at any missing/wrong-typed segment, so
// callers can blindly drill down without nil checks at each level.
func pick(v any, path ...any) any {
	for _, p := range path {
		switch step := p.(type) {
		case string:
			m, ok := v.(map[string]any)
			if !ok {
				return nil
			}
			v = m[step]
		case int:
			s, ok := v.([]any)
			if !ok || step < 0 || step >= len(s) {
				return nil
			}
			v = s[step]
		}
	}
	return v
}

func pickStr(v any, path ...any) string {
	s, _ := pick(v, path...).(string)
	return s
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
	req.Header.Set("User-Agent", "websitepapers/0.1 (mailto:pgribblle@uwo.ca)")

	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return Paper{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Paper{}, fmt.Errorf("DOI not found (status %d)", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Paper{}, err
	}
	m := body["message"]

	paper := Paper{
		DOI:     doi,
		Title:   pickStr(m, "title", 0),
		Journal: pickStr(m, "container-title", 0),
		Volume:  pickStr(m, "volume"),
		Pages:   pickStr(m, "page"),
	}
	if paper.Journal == "" {
		paper.Journal = pickStr(m, "institution", 0, "name")
	}
	if paper.Volume == "" && paper.Pages == "" {
		paper.Pages = pickStr(m, "article-number")
	}

	if list, ok := pick(m, "author").([]any); ok {
		names := make([]string, 0, len(list))
		for _, a := range list {
			name := pickStr(a, "family")
			if initials := givenInitials(pickStr(a, "given")); initials != "" {
				name += " " + initials
			}
			names = append(names, name)
		}
		paper.Authors = strings.Join(names, ", ")
	}

	for _, key := range []string{"published-print", "published-online", "issued"} {
		if y, ok := pick(m, key, "date-parts", 0, 0).(float64); ok {
			paper.Year = fmt.Sprintf("%d", int(y))
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
