package pubmed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestClient(baseURL string) *Client {
	cfg := DefaultConfig()
	cfg.BaseURL = baseURL
	cfg.Rate = 0
	return NewClient(cfg)
}

func TestGetSendsUserAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte(`{"esearchresult":{"count":"0","retmax":"0","idlist":[]}}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, _ = c.search(context.Background(), "test", 1)
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"esearchresult":{"count":"0","retmax":"0","idlist":[]}}`))
	}))
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 5
	c := NewClient(cfg)

	start := time.Now()
	_, err := c.search(context.Background(), "test", 1)
	if err != nil {
		t.Fatal(err)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestSearchReturnsArticles(t *testing.T) {
	const searchResp = `{"esearchresult":{"count":"1","retmax":"1","idlist":["12345678"]}}`
	const summaryResp = `{
		"result": {
			"uids": ["12345678"],
			"12345678": {
				"uid": "12345678",
				"pubdate": "2024 Jan",
				"source": "Nature",
				"authors": [{"name":"Doe J"},{"name":"Smith A"}],
				"title": "Test Article",
				"articleids": [{"idtype":"doi","value":"10.1038/test"}]
			}
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "esearch") {
			_, _ = w.Write([]byte(searchResp))
		} else {
			_, _ = w.Write([]byte(summaryResp))
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	arts, err := c.Search(context.Background(), "test", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 1 {
		t.Fatalf("got %d articles, want 1", len(arts))
	}
	a := arts[0]
	if a.PMID != "12345678" {
		t.Errorf("PMID = %q, want 12345678", a.PMID)
	}
	if a.DOI != "10.1038/test" {
		t.Errorf("DOI = %q, want 10.1038/test", a.DOI)
	}
	if a.Authors != "Doe J, Smith A" {
		t.Errorf("Authors = %q, want 'Doe J, Smith A'", a.Authors)
	}
	if a.Rank != 1 {
		t.Errorf("Rank = %d, want 1", a.Rank)
	}
}

func TestArticleReturnsRecord(t *testing.T) {
	const summaryResp = `{
		"result": {
			"uids": ["99999999"],
			"99999999": {
				"uid": "99999999",
				"pubdate": "2023 Jun",
				"source": "Science",
				"authors": [{"name":"Alpha A"},{"name":"Beta B"},{"name":"Gamma G"},{"name":"Delta D"}],
				"title": "Four Authors Paper",
				"articleids": []
			}
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(summaryResp))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	art, err := c.Article(context.Background(), "99999999")
	if err != nil {
		t.Fatal(err)
	}
	if art.Journal != "Science" {
		t.Errorf("Journal = %q, want Science", art.Journal)
	}
	if !strings.HasSuffix(art.Authors, "et al.") {
		t.Errorf("Authors = %q, expected et al. suffix", art.Authors)
	}
	wantURL := "https://pubmed.ncbi.nlm.nih.gov/99999999/"
	if art.URL != wantURL {
		t.Errorf("URL = %q, want %q", art.URL, wantURL)
	}
}

func TestAbstractParsesXML(t *testing.T) {
	const summaryResp = `{
		"result": {
			"uids": ["11111111"],
			"11111111": {
				"uid": "11111111",
				"pubdate": "2022",
				"source": "Lancet",
				"authors": [],
				"title": "Abstract Test",
				"articleids": []
			}
		}
	}`
	const abstractXML = `<?xml version="1.0" ?>
<!DOCTYPE PubmedArticleSet PUBLIC "-//NLM//DTD PubMedArticle, 1st January 2024//EN" "">
<PubmedArticleSet>
<PubmedArticle>
  <MedlineCitation>
    <Article>
      <Abstract>
        <AbstractText Label="BACKGROUND" NlmCategory="BACKGROUND">Some background.</AbstractText>
        <AbstractText Label="RESULTS" NlmCategory="RESULTS">Some results.</AbstractText>
      </Abstract>
    </Article>
  </MedlineCitation>
</PubmedArticle>
</PubmedArticleSet>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "efetch") {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(abstractXML))
		} else {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(summaryResp))
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	abs, err := c.Abstract(context.Background(), "11111111")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(abs.Abstract, "BACKGROUND: Some background.") {
		t.Errorf("abstract = %q, missing BACKGROUND section", abs.Abstract)
	}
	if !strings.Contains(abs.Abstract, "RESULTS: Some results.") {
		t.Errorf("abstract = %q, missing RESULTS section", abs.Abstract)
	}
}

func TestRelatedReturnsArticles(t *testing.T) {
	const elinkResp = `{
		"linksets": [{
			"dbfrom": "pubmed",
			"ids": [12345678],
			"linksetdbs": [{
				"dbto": "pubmed",
				"linkname": "pubmed_pubmed",
				"links": [
					{"id": "22222222", "score": "42000000"},
					{"id": "33333333", "score": "38000000"}
				]
			}]
		}]
	}`
	const summaryResp = `{
		"result": {
			"uids": ["22222222","33333333"],
			"22222222": {"uid":"22222222","pubdate":"2023","source":"Cell","authors":[],"title":"Related One","articleids":[]},
			"33333333": {"uid":"33333333","pubdate":"2022","source":"BMJ","authors":[],"title":"Related Two","articleids":[]}
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "elink") {
			_, _ = w.Write([]byte(elinkResp))
		} else {
			_, _ = w.Write([]byte(summaryResp))
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	arts, err := c.Related(context.Background(), "12345678", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 2 {
		t.Fatalf("got %d related articles, want 2", len(arts))
	}
	if arts[0].PMID != "22222222" {
		t.Errorf("first related PMID = %q, want 22222222", arts[0].PMID)
	}
}

func TestNotFoundOnEmptySearch(t *testing.T) {
	const searchResp = `{"esearchresult":{"count":"0","retmax":"0","idlist":[]}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(searchResp))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Search(context.Background(), "xyzzy-no-results", 5)
	if err != ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}
