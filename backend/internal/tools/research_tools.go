package tools

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"leadecho/internal/apikey"
	"leadecho/internal/treg"
)

// Research tools: look outside LeadEcho's own data so the assistant can
// answer "do they hire often?" or "is this company like our customers?"
// with evidence. Both go through the metered data provider (one credit a
// call) and return sources the answer can cite.

const (
	webSearchTool   = "treg.web.search"
	companyJobsTool = "anyapi.linkedin.search.jobs"
	maxWebResults   = 8
	maxJobResults   = 20
)

// Researcher is the slice of *treg.Client these tools use.
type Researcher interface {
	Enabled() bool
	Call(ctx context.Context, req treg.Request) (*treg.Response, error)
}

type WebResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

type CompanyJob struct {
	Title         string `json:"title"`
	Location      string `json:"location,omitempty"`
	URL           string `json:"url,omitempty"`
	PostedDaysAgo int    `json:"posted_days_ago"`
}

type CompanyJobsResult struct {
	Company string       `json:"company"`
	Window  string       `json:"window"`
	Count   int          `json:"count"`
	Jobs    []CompanyJob `json:"jobs"`
	Note    string       `json:"note"`
}

func researchTools(d Deps) []Tool {
	return []Tool{
		{
			Name:        "web_search",
			Description: "Search the public web. Use it for facts LeadEcho does not hold: what a company does, its size or funding, news about it, who its customers are, or who the user's own customers are. Returns titles, URLs and snippets; cite the URLs you rely on. Costs one credit.",
			Scope:       apikey.ScopeRead,
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["query"],"properties":{
"query":{"type":"string","minLength":2,"maxLength":300,"description":"What to search for, as you would type it into a search engine"}}}`),
			Handler: d.webSearch,
		},
		{
			Name:        "company_open_jobs",
			Description: "List the jobs one company posted in the past month (title, location, days ago). Use it to judge how actively and how often a company is hiring, and for which roles. Costs one credit.",
			Scope:       apikey.ScopeRead,
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["company"],"properties":{
"company":{"type":"string","minLength":2,"maxLength":120,"description":"Company name exactly as it appears on the lead"}}}`),
			Handler: d.companyOpenJobs,
		},
	}
}

func (d Deps) research(ctx context.Context, ws, tool string, body any) ([]byte, error) {
	if d.Research == nil || !d.Research.Enabled() {
		return nil, userErr("web research is not available on this server")
	}
	resp, err := d.Research.Call(ctx, treg.Request{WorkspaceID: ws, Tool: tool, Body: body})
	if err != nil {
		// Never forward provider details; say what the user can act on.
		return nil, userErr("the lookup could not be completed (paid data may be off for this workspace, or out of credits)")
	}
	return resp.Body, nil
}

func (d Deps) webSearch(ctx context.Context, ws string, args json.RawMessage) (any, error) {
	var in struct {
		Query string `json:"query"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	q := strings.TrimSpace(in.Query)
	if len(q) < 2 {
		return nil, userErr("query is required")
	}
	body, err := d.research(ctx, ws, webSearchTool, map[string]any{"q": q, "limit": maxWebResults})
	if err != nil {
		return nil, err
	}
	return map[string]any{"query": q, "results": parseWebResults(body)}, nil
}

// parseWebResults reads {output:{results:[…]}} (or unwrapped), keeps http(s)
// links only and caps count and snippet length: the text is third-party.
func parseWebResults(body []byte) []WebResult {
	type row struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Snippet string `json:"snippet"`
	}
	var v struct {
		Output *struct {
			Results []row `json:"results"`
		} `json:"output"`
		Results []row `json:"results"`
	}
	if json.Unmarshal(body, &v) != nil {
		return []WebResult{}
	}
	rows := v.Results
	if v.Output != nil {
		rows = v.Output.Results
	}
	out := make([]WebResult, 0, maxWebResults)
	for _, r := range rows {
		if len(out) == maxWebResults {
			break
		}
		if !strings.HasPrefix(r.URL, "https://") && !strings.HasPrefix(r.URL, "http://") {
			continue
		}
		out = append(out, WebResult{Title: clip(r.Title, 160), URL: r.URL, Snippet: clip(r.Snippet, 320)})
	}
	return out
}

func (d Deps) companyOpenJobs(ctx context.Context, ws string, args json.RawMessage) (any, error) {
	var in struct {
		Company string `json:"company"`
	}
	if err := decode(args, &in); err != nil {
		return nil, err
	}
	company := strings.TrimSpace(in.Company)
	if len(company) < 2 {
		return nil, userErr("company is required")
	}
	body, err := d.research(ctx, ws, companyJobsTool, map[string]string{"query": company, "postedLimit": "month"})
	if err != nil {
		return nil, err
	}
	jobs := parseCompanyJobs(body, company, time.Now())
	return CompanyJobsResult{
		Company: company, Window: "past month", Count: len(jobs), Jobs: jobs,
		Note: "Postings found on LinkedIn for this exact company name, first page of results only: treat the count as a minimum, and zero as 'none found', not 'not hiring'.",
	}, nil
}

// parseCompanyJobs keeps only postings by the named company: the search is by
// keyword, so it also returns other employers that mention the name.
func parseCompanyJobs(body []byte, company string, now time.Time) []CompanyJob {
	type row struct {
		Company    string `json:"company"`
		Title      string `json:"title"`
		Location   string `json:"location"`
		URL        string `json:"url"`
		CreatedUTC int64  `json:"createdUtc"`
	}
	type page struct {
		Data struct {
			Items []row `json:"items"`
		} `json:"data"`
	}
	var v struct {
		Output *page `json:"output"`
		page
	}
	if json.Unmarshal(body, &v) != nil {
		return []CompanyJob{}
	}
	items := v.Data.Items
	if v.Output != nil {
		items = v.Output.Data.Items
	}
	want := companyNameKey(company)
	out := make([]CompanyJob, 0, len(items))
	for _, it := range items {
		if len(out) == maxJobResults {
			break
		}
		if want == "" || companyNameKey(it.Company) != want {
			continue
		}
		days := 0
		if it.CreatedUTC > 0 {
			days = max(0, int(now.Sub(time.Unix(it.CreatedUTC, 0)).Hours()/24))
		}
		url := it.URL
		if !strings.HasPrefix(url, "https://") {
			url = ""
		}
		out = append(out, CompanyJob{Title: clip(it.Title, 140), Location: clip(it.Location, 80), URL: url, PostedDaysAgo: days})
	}
	return out
}

var (
	nameNoiseRe  = regexp.MustCompile(`\([^)]*\)`)
	nameSplitRe  = regexp.MustCompile(`[^a-z0-9]+`)
	nameSuffixes = map[string]bool{"inc": true, "llc": true, "ltd": true, "limited": true, "corp": true, "corporation": true, "co": true, "company": true, "gmbh": true, "plc": true}
)

// companyNameKey normalises a company name for an exact comparison: case,
// punctuation, bracketed notes and legal suffixes are ignored.
func companyNameKey(name string) string {
	words := strings.FieldsFunc(nameSplitRe.ReplaceAllString(strings.ToLower(nameNoiseRe.ReplaceAllString(name, " ")), " "), func(r rune) bool { return r == ' ' })
	for len(words) > 1 && nameSuffixes[words[len(words)-1]] {
		words = words[:len(words)-1]
	}
	return strings.Join(words, " ")
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
