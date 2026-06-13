package pubmed

// Article is the record emitted for PubMed articles from search, article, and related commands.
type Article struct {
	Rank    int    `json:"rank"`
	PMID    string `json:"pmid"`
	Title   string `json:"title"`
	Authors string `json:"authors"`
	Journal string `json:"journal"`
	Date    string `json:"date"`
	DOI     string `json:"doi"`
	URL     string `json:"url"`
}

// Abstract holds the parsed abstract text for one article.
type Abstract struct {
	PMID     string `json:"pmid"`
	Title    string `json:"title"`
	Authors  string `json:"authors"`
	Journal  string `json:"journal"`
	Date     string `json:"date"`
	Abstract string `json:"abstract"`
	URL      string `json:"url"`
}

// ─── wire types ──────────────────────────────────────────────────────────────

type esearchResp struct {
	ESearchResult struct {
		Count  string   `json:"count"`
		RetMax string   `json:"retmax"`
		IDList []string `json:"idlist"`
	} `json:"esearchresult"`
}

type summaryDoc struct {
	UID        string       `json:"uid"`
	PubDate    string       `json:"pubdate"`
	Source     string       `json:"source"`
	Authors    []authorItem `json:"authors"`
	LastAuthor string       `json:"lastauthor"`
	Title      string       `json:"title"`
	PubType    []string     `json:"pubtype"`
	ArticleIDs []articleID  `json:"articleids"`
}

type authorItem struct {
	Name string `json:"name"`
}

type articleID struct {
	IDType string `json:"idtype"`
	Value  string `json:"value"`
}

type elinkResp struct {
	LinkSets []elinkSet `json:"linksets"`
}

type elinkSet struct {
	LinkSetDBs []elinkDB `json:"linksetdbs"`
}

type elinkDB struct {
	LinkName string     `json:"linkname"`
	Links    []linkItem `json:"links"`
}

type linkItem struct {
	ID    string `json:"id"`
	Score string `json:"score"`
}

// ─── XML types for efetch abstract ───────────────────────────────────────────

type pubmedArticleSet struct {
	Articles []pubmedArticle `xml:"PubmedArticle"`
}

type pubmedArticle struct {
	MedlineCitation medlineCitation `xml:"MedlineCitation"`
}

type medlineCitation struct {
	Article medlineArticle `xml:"Article"`
}

type medlineArticle struct {
	AbstractTexts []abstractText `xml:"Abstract>AbstractText"`
}

type abstractText struct {
	Label string `xml:"Label,attr"`
	Text  string `xml:",chardata"`
}
