package main

import (
	"regexp"
	"strings"
)

var doiRegex = regexp.MustCompile(`(?i)^10\.\d{4,}(?:\.\d+)?/\S+$`)

// normalizeDOI strips known doi.org URL prefixes (case-insensitive) and
// lowercases the result. DOIs are officially case-insensitive, so storage
// and comparison use the lowercase form.
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
