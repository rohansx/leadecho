package onboarding

import (
	"context"
	"net/url"
	"strings"
	"unicode"

	"leadecho/internal/monitor"
)

// CompetitorFinder is the fallback used when the product analysis returns no
// competitors. Implementations must be nil-safe on the caller side: a nil
// finder simply means "no fallback".
//
// Hook: a treg web-search finder (treg.web.search "<product> alternatives",
// plan P1 #8) slots in here behind the same interface once internal/treg exists.
type CompetitorFinder interface {
	FindCompetitors(ctx context.Context, product, productURL string) ([]string, error)
}

// ExaCompetitorFinder searches Exa's company index for "<product> alternatives".
type ExaCompetitorFinder struct {
	APIKey   string
	Endpoint string // "" = production Exa API
}

// NewCompetitorFinder returns an Exa-backed finder, or nil when no key is set.
func NewCompetitorFinder(exaAPIKey string) CompetitorFinder {
	if exaAPIKey == "" {
		return nil
	}
	return &ExaCompetitorFinder{APIKey: exaAPIKey}
}

const maxFoundCompetitors = 8

// FindCompetitors returns up to 8 competitor names; nil without a key.
func (f *ExaCompetitorFinder) FindCompetitors(ctx context.Context, product, productURL string) ([]string, error) {
	if f == nil || f.APIKey == "" || strings.TrimSpace(product) == "" {
		return nil, nil
	}
	results, err := monitor.ExaWebSearch(ctx, f.Endpoint, f.APIKey, product+" alternatives", "company", 15)
	if err != nil {
		return nil, err
	}
	ownHost := hostOf(productURL)
	var names []string
	seenHost := map[string]bool{}
	for _, r := range results {
		host := hostOf(r.URL)
		if seenHost[host] {
			continue // one name per company site (homepage + pricing page etc.)
		}
		if n := competitorName(r.Title, r.URL, product, ownHost); n != "" {
			seenHost[host] = true
			names = append(names, n)
		}
	}
	return cleanList(names, maxFoundCompetitors), nil
}

// aggregatorHosts are review/listicle sites that show up for "X alternatives"
// but are not competitors.
var aggregatorHosts = []string{
	"g2.com", "capterra.com", "alternativeto.net", "getapp.com", "softwareadvice.com",
	"trustradius.com", "producthunt.com", "saasworthy.com", "slashdot.org", "sourceforge.net",
	"reddit.com", "medium.com", "youtube.com", "quora.com", "linkedin.com", "wikipedia.org",
	"crunchbase.com", "forbes.com", "techradar.com", "zapier.com",
}

// competitorName derives a company name from a company-search hit: the title's
// first segment, else the domain label. Empty for aggregators and the product itself.
func competitorName(title, rawURL, product, ownHost string) string {
	host := hostOf(rawURL)
	if host == "" {
		return ""
	}
	for _, a := range aggregatorHosts {
		if host == a || strings.HasSuffix(host, "."+a) {
			return ""
		}
	}
	if ownHost != "" && (host == ownHost || strings.HasSuffix(host, "."+ownHost)) {
		return ""
	}
	name := strings.TrimSpace(title)
	if i := strings.IndexAny(name, "-|:–—·"); i >= 0 {
		name = strings.TrimSpace(name[:i])
	}
	label := strings.SplitN(host, ".", 2)[0]
	// Long titles are headlines, not names; fall back to the domain.
	if name == "" || len(strings.Fields(name)) > 3 {
		name = titleCase(label)
	}
	if strings.EqualFold(name, product) {
		return ""
	}
	return name
}

func hostOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
