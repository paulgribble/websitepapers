package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockCrossrefHandler points crossrefBase at an httptest server with the
// given handler, restoring the original on test cleanup.
func mockCrossrefHandler(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	orig := crossrefBase
	t.Cleanup(func() { crossrefBase = orig })
	crossrefBase = srv.URL
}

// mockCrossrefBody is the common case: respond with a fixed JSON body.
func mockCrossrefBody(t *testing.T, body string) {
	t.Helper()
	mockCrossrefHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(body))
	})
}

func TestNormalizeDOI(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"10.1038/abc", "10.1038/abc"},
		{"10.1038/ABC", "10.1038/abc"},
		{"  10.1038/abc  ", "10.1038/abc"},
		{"https://doi.org/10.1038/abc", "10.1038/abc"},
		{"http://doi.org/10.1038/abc", "10.1038/abc"},
		{"https://dx.doi.org/10.1038/abc", "10.1038/abc"},
		{"http://dx.doi.org/10.1038/abc", "10.1038/abc"},
		{"dx.doi.org/10.1038/abc", "10.1038/abc"},
		{"doi.org/10.1038/abc", "10.1038/abc"},
		{"HTTPS://DOI.ORG/10.1038/ABC", "10.1038/abc"},
		{"Https://Doi.Org/10.1038/AbC", "10.1038/abc"},
	}
	for _, c := range cases {
		got := normalizeDOI(c.in)
		if got != c.want {
			t.Errorf("normalizeDOI(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCitationText(t *testing.T) {
	cases := []struct {
		name string
		p    Paper
		want string
	}{
		{"no journal", Paper{DOI: "10.1/x"}, "10.1/x"},
		{"biorxiv lower", Paper{DOI: "10.1101/2026.04.27.721195", Journal: "bioRxiv"}, "bioRxiv:2026.04.27.721195"},
		{"biorxiv mixed case", Paper{DOI: "10.1101/2026.04.27.721195", Journal: "BioRxiv"}, "BioRxiv:2026.04.27.721195"},
		{"medrxiv", Paper{DOI: "10.1101/abc", Journal: "medRxiv"}, "medRxiv:abc"},
		{"vol+pages", Paper{Journal: "J Neurophysiol", Volume: "135", Pages: "1175-1185"}, "J Neurophysiol 135:1175-1185"},
		{"vol only", Paper{Journal: "Nature", Volume: "600"}, "Nature 600"},
		{"pages only", Paper{Journal: "Nature", Pages: "12"}, "Nature 12"},
		{"journal only", Paper{Journal: "Nature"}, "Nature"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := citationText(c.p); got != c.want {
				t.Errorf("citationText() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestFetchMetadata(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mockCrossrefHandler(t, func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/works/") {
				t.Errorf("unexpected path %q", r.URL.Path)
			}
			if ua := r.Header.Get("User-Agent"); !strings.Contains(ua, "websitepapers") {
				t.Errorf("missing/unexpected User-Agent: %q", ua)
			}
			w.Write([]byte(`{"message":{
                "title":["Demo Title"],
                "container-title":["J Demo"],
                "volume":"42",
                "page":"1-10",
                "author":[{"given":"Alice","family":"Smith"},{"given":"Paul L","family":"Gribble"},{"given":"Émile","family":"Zola"}],
                "published-print":{"date-parts":[[2024,5,1]]}
            }}`))
		})

		p, err := fetchMetadata("10.1/x")
		if err != nil {
			t.Fatal(err)
		}
		if p.Title != "Demo Title" || p.Journal != "J Demo" || p.Volume != "42" ||
			p.Pages != "1-10" || p.Year != "2024" || p.Authors != "Smith A., Gribble P. L., Zola É." {
			t.Errorf("unexpected paper: %+v", p)
		}
	})

	t.Run("404", func(t *testing.T) {
		mockCrossrefHandler(t, func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "nope", http.StatusNotFound)
		})

		if _, err := fetchMetadata("10.1/x"); err == nil {
			t.Fatal("expected error on 404")
		}
	})

	t.Run("article-number fallback", func(t *testing.T) {
		mockCrossrefBody(t, `{"message":{
            "title":["T"],"container-title":["J"],
            "article-number":"e12345",
            "issued":{"date-parts":[[2025]]}
        }}`)

		p, err := fetchMetadata("10.1/x")
		if err != nil {
			t.Fatal(err)
		}
		if p.Pages != "e12345" {
			t.Errorf("expected article-number fallback in Pages, got %q", p.Pages)
		}
		if p.Year != "2025" {
			t.Errorf("expected issued year 2025, got %q", p.Year)
		}
	})

	t.Run("biorxiv institution fallback", func(t *testing.T) {
		mockCrossrefBody(t, `{"message":{
            "title":["Preprint Title"],
            "container-title":[],
            "institution":[{"name":"bioRxiv"}],
            "type":"posted-content",
            "subtype":"preprint",
            "issued":{"date-parts":[[2020]]}
        }}`)

		p, err := fetchMetadata("10.1101/2020.03.25.008466")
		if err != nil {
			t.Fatal(err)
		}
		if p.Journal != "bioRxiv" {
			t.Errorf("expected institution fallback Journal=bioRxiv, got %q", p.Journal)
		}
	})

	t.Run("year falls through to published-online", func(t *testing.T) {
		mockCrossrefBody(t, `{"message":{
            "title":["T"],"container-title":["J"],
            "published-online":{"date-parts":[[2023,2,3]]},
            "issued":{"date-parts":[[2022]]}
        }}`)

		p, err := fetchMetadata("10.1/x")
		if err != nil {
			t.Fatal(err)
		}
		if p.Year != "2023" {
			t.Errorf("expected published-online year 2023, got %q", p.Year)
		}
	})

	t.Run("trailing slash on base", func(t *testing.T) {
		mockCrossrefBody(t, `{"message":{"title":["T"]}}`)
		crossrefBase = crossrefBase + "/"

		if _, err := fetchMetadata("10.1/x"); err != nil {
			t.Fatalf("trailing slash on base broke fetch: %v", err)
		}
	})
}

func TestGivenInitials(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"Paul", "P."},
		{"Paul L", "P. L."},
		{"Paul L.", "P. L."},
		{"Paul Luc", "P. L."},
		{"  Paul   L  ", "P. L."},
		{"Émile", "É."},
		{"J", "J."},
		{"Andrew A.G.", "A. A. G."},
		{"A.G.", "A. G."},
		{"A. G.", "A. G."},
		{"Marie-Claude", "M. C."},
	}
	for _, c := range cases {
		if got := givenInitials(c.in); got != c.want {
			t.Errorf("givenInitials(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBibAuthors(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"Smith", "Smith"},
		{"Smith J.", "Smith, J."},
		{"Smith J., Jones A.", "Smith, J. and Jones, A."},
		{"Smith J., Jones A., Doe X.", "Smith, J. and Jones, A. and Doe, X."},
		{"Gribble P. L.", "Gribble, P. L."},
		{"van der Berg J.", "van der Berg, J."},
		{"van der Berg P. L.", "van der Berg, P. L."},
		{"Zola É.", "Zola, É."},
		{"Smith J., Gribble P. L.", "Smith, J. and Gribble, P. L."},
	}
	for _, c := range cases {
		if got := bibAuthors(c.in); got != c.want {
			t.Errorf("bibAuthors(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBibEscape(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"plain text", "plain text"},
		{"50% off & free", `50\% off \& free`},
		{"a_b#c$d", `a\_b\#c\$d`},
		{"{x}", `\{x\}`},
	}
	for _, c := range cases {
		if got := bibEscape(c.in); got != c.want {
			t.Errorf("bibEscape(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBibAsciiFold(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"ASCII", "ASCII"},
		{"Müller", "Muller"},
		{"Émile", "Emile"},
		{"Zoë", "Zoe"},
		{"naïve", "naive"},
		{"Çelik", "Celik"},
	}
	for _, c := range cases {
		if got := bibAsciiFold(c.in); got != c.want {
			t.Errorf("bibAsciiFold(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBibKey(t *testing.T) {
	used := map[string]int{}
	p1 := Paper{Authors: "Smith J., Jones A.", Year: "2024", Title: "A Cool Paper About Stuff"}
	if got := bibKey(p1, used); got != "smith2024a" {
		t.Errorf("first key = %q, want %q", got, "smith2024a")
	}
	if got := bibKey(p1, used); got != "smith2024a_2" {
		t.Errorf("second key = %q, want %q", got, "smith2024a_2")
	}

	p2 := Paper{}
	if got := bibKey(p2, used); got != "paper" {
		t.Errorf("empty paper key = %q, want %q", got, "paper")
	}

	p3 := Paper{Authors: "Müller H.", Year: "2024", Title: "On X"}
	if got := bibKey(p3, used); got != "muller2024on" {
		t.Errorf("diacritic key = %q, want %q", got, "muller2024on")
	}
}
