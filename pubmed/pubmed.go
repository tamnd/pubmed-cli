// Package pubmed is the library behind the pubmed command: the HTTP client,
// request shaping, and the typed data models for PubMed.
//
// All data comes from the NCBI E-utilities REST API at
// https://eutils.ncbi.nlm.nih.gov/entrez/eutils. No API key is required.
// The default rate of 400ms between requests stays under the 3 req/s limit
// for anonymous access.
package pubmed

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultUserAgent identifies the client to NCBI.
const DefaultUserAgent = "pubmed/dev (+https://github.com/tamnd/pubmed-cli)"

// ErrNotFound is returned when a PMID or result set yields no data.
var ErrNotFound = errors.New("not found")

// Config holds constructor parameters.
type Config struct {
	BaseURL   string
	UserAgent string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
}

// DefaultConfig returns sensible defaults for anonymous NCBI access.
func DefaultConfig() Config {
	return Config{
		BaseURL:   "https://eutils.ncbi.nlm.nih.gov/entrez/eutils",
		UserAgent: DefaultUserAgent,
		Rate:      400 * time.Millisecond,
		Retries:   5,
		Timeout:   30 * time.Second,
	}
}

// Client talks to the NCBI E-utilities API.
type Client struct {
	httpClient *http.Client
	baseURL    string
	userAgent  string
	rate       time.Duration
	retries    int
	mu         sync.Mutex
	last       time.Time
}

// NewClient returns a Client with the given config.
func NewClient(cfg Config) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: cfg.Timeout},
		baseURL:    cfg.BaseURL,
		userAgent:  cfg.UserAgent,
		rate:       cfg.Rate,
		retries:    cfg.Retries,
	}
}

// Search searches PubMed for articles matching query and returns up to limit Article records.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Article, error) {
	pmids, err := c.search(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	if len(pmids) == 0 {
		return nil, ErrNotFound
	}
	return c.summary(ctx, pmids, 1)
}

// Article returns the Article record for a single PMID.
func (c *Client) Article(ctx context.Context, pmid string) (Article, error) {
	arts, err := c.summary(ctx, []string{pmid}, 1)
	if err != nil {
		return Article{}, err
	}
	if len(arts) == 0 {
		return Article{}, ErrNotFound
	}
	return arts[0], nil
}

// Abstract fetches and returns the full abstract for a PMID.
func (c *Client) Abstract(ctx context.Context, pmid string) (Abstract, error) {
	// Get metadata first.
	art, err := c.Article(ctx, pmid)
	if err != nil {
		return Abstract{}, err
	}

	// Fetch abstract XML.
	u := c.buildURL("/efetch.fcgi", url.Values{
		"db":      {"pubmed"},
		"id":      {pmid},
		"retmode": {"xml"},
		"rettype": {"abstract"},
	})
	body, err := c.get(ctx, u)
	if err != nil {
		return Abstract{}, err
	}

	var set pubmedArticleSet
	if err := xml.Unmarshal(body, &set); err != nil {
		return Abstract{}, fmt.Errorf("decode abstract xml: %w", err)
	}
	if len(set.Articles) == 0 {
		return Abstract{}, ErrNotFound
	}

	texts := set.Articles[0].MedlineCitation.Article.AbstractTexts
	absText := joinAbstract(texts)

	return Abstract{
		PMID:     art.PMID,
		Title:    art.Title,
		Authors:  art.Authors,
		Journal:  art.Journal,
		Date:     art.Date,
		Abstract: absText,
		URL:      art.URL,
	}, nil
}

// Related returns articles related to the given PMID via NCBI link scoring.
func (c *Client) Related(ctx context.Context, pmid string, limit int) ([]Article, error) {
	u := c.buildURL("/elink.fcgi", url.Values{
		"dbfrom":  {"pubmed"},
		"db":      {"pubmed"},
		"cmd":     {"neighbor_score"},
		"id":      {pmid},
		"retmode": {"json"},
	})
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var resp elinkResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode elink: %w", err)
	}

	var pmids []string
	for _, ls := range resp.LinkSets {
		for _, db := range ls.LinkSetDBs {
			if db.LinkName == "pubmed_pubmed" {
				for _, lk := range db.Links {
					pmids = append(pmids, lk.ID)
					if limit > 0 && len(pmids) >= limit {
						break
					}
				}
			}
		}
	}
	if len(pmids) == 0 {
		return nil, ErrNotFound
	}
	return c.summary(ctx, pmids, 1)
}

// ─── internal helpers ─────────────────────────────────────────────────────────

func (c *Client) search(ctx context.Context, query string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 20
	}
	u := c.buildURL("/esearch.fcgi", url.Values{
		"db":         {"pubmed"},
		"term":       {query},
		"retmax":     {fmt.Sprintf("%d", limit)},
		"retmode":    {"json"},
		"usehistory": {"y"},
	})
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var resp esearchResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode esearch: %w", err)
	}
	return resp.ESearchResult.IDList, nil
}

func (c *Client) summary(ctx context.Context, pmids []string, startRank int) ([]Article, error) {
	if len(pmids) == 0 {
		return nil, nil
	}
	// Batch up to 200 at a time.
	const batchSize = 200
	var out []Article
	rank := startRank
	for i := 0; i < len(pmids); i += batchSize {
		end := i + batchSize
		if end > len(pmids) {
			end = len(pmids)
		}
		batch := pmids[i:end]
		arts, err := c.summaryBatch(ctx, batch, rank)
		if err != nil {
			return out, err
		}
		out = append(out, arts...)
		rank += len(arts)
	}
	return out, nil
}

func (c *Client) summaryBatch(ctx context.Context, pmids []string, startRank int) ([]Article, error) {
	u := c.buildURL("/esummary.fcgi", url.Values{
		"db":      {"pubmed"},
		"id":      {strings.Join(pmids, ",")},
		"retmode": {"json"},
	})
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}

	// The result map has a "uids" key (array) plus one key per PMID.
	// We unmarshal result into map[string]json.RawMessage to handle the
	// heterogeneous "uids" key, then decode only the PMID entries.
	var raw struct {
		Result map[string]json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode esummary: %w", err)
	}

	// Read uid ordering from the "uids" key.
	var uids []string
	if v, ok := raw.Result["uids"]; ok {
		if err := json.Unmarshal(v, &uids); err != nil {
			return nil, fmt.Errorf("decode esummary uids: %w", err)
		}
	} else {
		// Fallback: use the pmids we requested (order may differ).
		uids = pmids
	}

	out := make([]Article, 0, len(uids))
	rank := startRank
	for _, uid := range uids {
		v, ok := raw.Result[uid]
		if !ok {
			continue
		}
		var doc summaryDoc
		if err := json.Unmarshal(v, &doc); err != nil {
			continue
		}
		if doc.UID == "" {
			continue
		}
		out = append(out, docToArticle(doc, rank))
		rank++
	}
	return out, nil
}

func (c *Client) buildURL(path string, params url.Values) string {
	// Append NCBI courtesy parameters.
	params.Set("tool", "pubmed-cli")
	params.Set("email", "tamnd87@gmail.com")
	return c.baseURL + path + "?" + params.Encode()
}

func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

func (c *Client) pace() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rate <= 0 {
		return
	}
	if wait := c.rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// ─── conversion helpers ───────────────────────────────────────────────────────

func docToArticle(doc summaryDoc, rank int) Article {
	return Article{
		Rank:    rank,
		PMID:    doc.UID,
		Title:   doc.Title,
		Authors: formatAuthors(doc.Authors),
		Journal: doc.Source,
		Date:    doc.PubDate,
		DOI:     extractDOI(doc.ArticleIDs),
		URL:     "https://pubmed.ncbi.nlm.nih.gov/" + doc.UID + "/",
	}
}

func formatAuthors(authors []authorItem) string {
	if len(authors) == 0 {
		return ""
	}
	names := make([]string, 0, 3)
	for i, a := range authors {
		if i >= 3 {
			break
		}
		names = append(names, a.Name)
	}
	s := strings.Join(names, ", ")
	if len(authors) > 3 {
		s += " et al."
	}
	return s
}

func extractDOI(ids []articleID) string {
	for _, a := range ids {
		if a.IDType == "doi" {
			return a.Value
		}
	}
	return ""
}

func joinAbstract(texts []abstractText) string {
	var parts []string
	for _, t := range texts {
		s := strings.TrimSpace(t.Text)
		if s == "" {
			continue
		}
		if t.Label != "" {
			s = t.Label + ": " + s
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "\n")
}
