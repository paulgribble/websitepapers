package main

import (
	"fmt"
	"io"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var asciiFolder = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// bibAsciiFold strips Unicode combining marks (accents, umlauts, etc.) so
// that "Müller" becomes "Muller" and "Émile" becomes "Emile". Used for
// citation key generation only; displayed author text is unaffected.
func bibAsciiFold(s string) string {
	out, _, _ := transform.String(asciiFolder, s)
	return out
}

// writeBibEntry writes one @article{...} block to w. used tracks emitted
// citation keys so collisions get _2, _3 suffixes within a single export.
func writeBibEntry(w io.Writer, p Paper, used map[string]int) {
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
	for _, word := range strings.Fields(p.Title) {
		if alpha := bibAlpha(word); alpha != "" {
			titleWord = alpha
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

// bibAlpha keeps only ASCII letters and digits, after folding Latin
// diacritics to their base letters.
func bibAlpha(s string) string {
	s = bibAsciiFold(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// bibAuthors converts the stored "Family I., Family I." form to the
// "Family, I. and Family, I." form that BibTeX parses correctly. The split
// point is the first whitespace-separated token shaped like an initial
// (<rune>.) so multi-word surnames ("van der Berg") stay intact.
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
