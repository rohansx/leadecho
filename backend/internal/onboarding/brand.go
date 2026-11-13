// Package onboarding holds the logic behind the onboarding flow: the brand
// profile, the listener builder, the competitor fallback, the KB seed crawl
// and the calibration examples collected from sample feedback.
package onboarding

import (
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"

	"leadecho/internal/ai"
	"leadecho/internal/database"
)

const (
	maxTextLen  = 2000
	maxItemLen  = 200
	maxListLen  = 30
	maxKeywords = 25 // per group
)

// BrandProfile is the editable, persisted product analysis (API shape).
type BrandProfile struct {
	Overview         string               `json:"overview"`
	Product          string               `json:"product"`
	Positioning      string               `json:"positioning"`
	CustomerSegments []ai.CustomerSegment `json:"customer_segments"`
	Competitors      []string             `json:"competitors"`
	PainPoints       []ai.PainPointDetail `json:"pain_points"`
	Keywords         ai.KeywordGroups     `json:"keywords"`
	Features         []string             `json:"features"`
	TargetAudience   string               `json:"target_audience"`
	SourceURL        string               `json:"source_url"`
	UpdatedAt        *time.Time           `json:"updated_at,omitempty"`
}

// FromAnalysis builds a brand profile from a product analysis, deriving the
// extended fields (segments, detailed pain points, keyword groups) from the
// older flat fields when the model did not return them.
func FromAnalysis(pa *ai.ProductAnalysis, sourceURL string) BrandProfile {
	bp := BrandProfile{
		Overview:         pa.Description,
		Product:          pa.ProductName,
		Positioning:      pa.Positioning,
		CustomerSegments: pa.CustomerSegments,
		Competitors:      cleanList(pa.Competitors, maxListLen),
		PainPoints:       pa.PainPointDetails,
		Keywords:         pa.KeywordGroups,
		Features:         pa.Features,
		TargetAudience:   string(pa.TargetAudience),
		SourceURL:        sourceURL,
	}
	if len(bp.CustomerSegments) == 0 {
		for _, name := range cleanList(strings.Split(bp.TargetAudience, ","), 5) {
			bp.CustomerSegments = append(bp.CustomerSegments, ai.CustomerSegment{Name: name})
		}
	}
	if len(bp.PainPoints) == 0 {
		for _, p := range cleanList(pa.PainPoints, maxListLen) {
			bp.PainPoints = append(bp.PainPoints, ai.PainPointDetail{Problem: p})
		}
	}
	if keywordCount(bp.Keywords) == 0 {
		bp.Keywords = deriveKeywordGroups(pa, bp.Competitors)
	}
	return bp
}

// deriveKeywordGroups sorts flat suggested keywords into the four groups.
func deriveKeywordGroups(pa *ai.ProductAnalysis, competitors []string) ai.KeywordGroups {
	kg := ai.KeywordGroups{}
	name := strings.ToLower(strings.TrimSpace(pa.ProductName))
	if name != "" {
		kg.Brand = append(kg.Brand, pa.ProductName)
	}
	isCompetitor := map[string]bool{}
	for _, c := range competitors {
		isCompetitor[strings.ToLower(c)] = true
	}
	for _, k := range cleanList(pa.SuggestedKeywords, maxListLen) {
		lk := strings.ToLower(k)
		switch {
		case isCompetitor[lk]:
			// competitors have their own tab
		case name != "" && strings.Contains(lk, name):
			kg.Brand = append(kg.Brand, k)
		default:
			kg.Category = append(kg.Category, k)
		}
	}
	kg.Product = cleanList(pa.Features, 8)
	kg.Problem = cleanList(pa.PainPoints, 10)
	return kg
}

func keywordCount(kg ai.KeywordGroups) int {
	return len(kg.Brand) + len(kg.Product) + len(kg.Category) + len(kg.Problem)
}

// AllKeywords flattens the groups, deduped.
func (bp BrandProfile) AllKeywords() []string {
	k := bp.Keywords
	all := append(append(append(append([]string{}, k.Brand...), k.Product...), k.Category...), k.Problem...)
	return cleanList(all, 4*maxKeywords)
}

// Normalized trims, dedupes and caps every field, and validates the profile.
func (bp BrandProfile) Normalized() (BrandProfile, error) {
	out := bp
	out.Product = capLen(strings.TrimSpace(bp.Product), maxItemLen)
	if out.Product == "" {
		return out, errors.New("product is required")
	}
	out.SourceURL = strings.TrimSpace(bp.SourceURL)
	if out.SourceURL != "" && !isHTTPURL(out.SourceURL) {
		return out, errors.New("source_url must be an http(s) URL")
	}
	out.Overview = capLen(strings.TrimSpace(bp.Overview), maxTextLen)
	out.Positioning = capLen(strings.TrimSpace(bp.Positioning), maxTextLen)
	out.TargetAudience = capLen(strings.TrimSpace(bp.TargetAudience), maxTextLen)
	out.Competitors = cleanList(bp.Competitors, maxListLen)
	out.Features = cleanList(bp.Features, maxListLen)
	out.Keywords = ai.KeywordGroups{
		Brand:    cleanList(bp.Keywords.Brand, maxKeywords),
		Product:  cleanList(bp.Keywords.Product, maxKeywords),
		Category: cleanList(bp.Keywords.Category, maxKeywords),
		Problem:  cleanList(bp.Keywords.Problem, maxKeywords),
	}
	out.CustomerSegments = nil
	for _, s := range bp.CustomerSegments {
		if seg, ok := normalizeSegment(s); ok && len(out.CustomerSegments) < maxListLen {
			out.CustomerSegments = append(out.CustomerSegments, seg)
		}
	}
	out.PainPoints = nil
	for _, p := range bp.PainPoints {
		if prob := capLen(strings.TrimSpace(p.Problem), maxItemLen); prob != "" && len(out.PainPoints) < maxListLen {
			out.PainPoints = append(out.PainPoints, ai.PainPointDetail{Problem: prob, Explanation: capLen(strings.TrimSpace(p.Explanation), maxTextLen)})
		}
	}
	return out, nil
}

const maxSegmentList = 5 // criteria, negative criteria, examples per segment

var countryCodeRe = regexp.MustCompile(`^[A-Z]{2}$`)

// normalizeSegment trims and caps one segment; ok is false without a name.
// The fields reach search providers and prompts, so each is bounded.
func normalizeSegment(s ai.CustomerSegment) (ai.CustomerSegment, bool) {
	name := capLen(strings.TrimSpace(s.Name), maxItemLen)
	if name == "" {
		return ai.CustomerSegment{}, false
	}
	country := strings.ToUpper(strings.TrimSpace(s.Country))
	if !countryCodeRe.MatchString(country) {
		country = ""
	}
	return ai.CustomerSegment{
		Name:             name,
		WhyTheyBuy:       capLen(strings.TrimSpace(s.WhyTheyBuy), maxTextLen),
		Pain:             capLen(strings.TrimSpace(s.Pain), maxTextLen),
		Criteria:         omitEmpty(cleanList(s.Criteria, maxSegmentList)),
		NegativeCriteria: omitEmpty(cleanList(s.NegativeCriteria, maxSegmentList)),
		ExampleCompanies: omitEmpty(cleanList(s.ExampleCompanies, maxSegmentList)),
		BuyerTitles:      capLen(strings.TrimSpace(s.BuyerTitles), maxItemLen),
		Country:          country,
	}, true
}

// omitEmpty keeps an empty list out of the stored JSON (omitempty needs nil).
func omitEmpty(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}

// Params converts to the upsert parameters (nil slices become empty).
func (bp BrandProfile) Params(workspaceID string) database.UpsertBrandProfileParams {
	return database.UpsertBrandProfileParams{
		WorkspaceID:      workspaceID,
		Overview:         bp.Overview,
		Product:          bp.Product,
		Positioning:      bp.Positioning,
		CustomerSegments: mustJSON(nonNil(bp.CustomerSegments)),
		Competitors:      nonNil(bp.Competitors),
		PainPoints:       mustJSON(nonNil(bp.PainPoints)),
		Keywords:         mustJSON(bp.Keywords),
		Features:         nonNil(bp.Features),
		TargetAudience:   bp.TargetAudience,
		SourceUrl:        bp.SourceURL,
	}
}

// FromRow converts a stored row to the API shape.
func FromRow(r database.BrandProfile) BrandProfile {
	bp := BrandProfile{
		Overview:       r.Overview,
		Product:        r.Product,
		Positioning:    r.Positioning,
		Competitors:    r.Competitors,
		Features:       r.Features,
		TargetAudience: r.TargetAudience,
		SourceURL:      r.SourceUrl,
	}
	if !r.UpdatedAt.IsZero() {
		t := r.UpdatedAt
		bp.UpdatedAt = &t
	}
	// Stored by Params, so malformed JSON only means an empty field.
	_ = json.Unmarshal(r.CustomerSegments, &bp.CustomerSegments)
	_ = json.Unmarshal(r.PainPoints, &bp.PainPoints)
	_ = json.Unmarshal(r.Keywords, &bp.Keywords)
	return bp
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// cleanList trims, drops empties, dedupes case-insensitively and caps.
func cleanList(in []string, max int) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		s = capLen(strings.TrimSpace(s), maxItemLen)
		k := strings.ToLower(s)
		if s == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, s)
		if len(out) == max {
			break
		}
	}
	return out
}

func capLen(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func isHTTPURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
