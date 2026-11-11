package digest

import (
	"fmt"
	"sort"
	"strings"
)

// Noisy keyword thresholds: many mentions, low average fit (the scorer's
// "filtered" band is < 4.0).
const (
	NoisyMinMentions = 10
	NoisyMaxAvgScore = 4.0
	maxNoisyKeywords = 5
	MaxTopLeads      = 10
)

// Lead is one high-fit mention in the digest.
type Lead struct {
	MentionID string
	Who       string // name · role · company, when enrichment found them
	Draft     string // ready-to-post reply (daily digest, top leads only)
	Title     string
	URL       string
	Platform  string
	Author    string
	Intent    string
	Score     float64
}

type Count struct {
	Label string
	N     int
}

type KeywordStat struct {
	Term     string
	Mentions int
	AvgScore float64
}

// Data is everything a digest shows for one period. It is pure data so the
// builder, summary and renderers are testable without a database.
type Data struct {
	Kind          Kind
	Period        Period
	TopLeads      []Lead
	Platforms     []Count
	Intents       []Count
	NoisyKeywords []KeywordStat
	RepliesPosted int
	Clicks        int
	Signups       int
	RevenueCents  int64
	// Pipeline is where every lead stands today (not just this period): the
	// running proof of what the leads turned into.
	Pipeline     []Count
	Summary      string // weekly AI (or template) paragraph
	DashboardURL string
}

// NoisyKeywords picks keywords with many mentions but low fit, loudest first.
func NoisyKeywords(stats []KeywordStat) []KeywordStat {
	out := []KeywordStat{}
	for _, k := range stats {
		if k.Mentions >= NoisyMinMentions && k.AvgScore < NoisyMaxAvgScore {
			out = append(out, k)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Mentions > out[j].Mentions })
	if len(out) > maxNoisyKeywords {
		out = out[:maxNoisyKeywords]
	}
	return out
}

// pipelineLabels names lead stages the way the Pipeline page does, in order.
var pipelineLabels = []struct{ stage, label string }{
	{"prospect", "to reply"}, {"qualified", "replied"}, {"engaged", "in conversation"}, {"converted", "won"}, {"lost", "lost"},
}

// PipelineCounts orders raw stage counts and drops empty stages.
func PipelineCounts(byStage map[string]int) []Count {
	out := []Count{}
	for _, s := range pipelineLabels {
		if n := byStage[s.stage]; n > 0 {
			out = append(out, Count{s.label, n})
		}
	}
	return out
}

func sumCounts(cs []Count) int {
	n := 0
	for _, c := range cs {
		n += c.N
	}
	return n
}

func (d Data) intentCount(labels ...string) int {
	n := 0
	for _, c := range d.Intents {
		for _, l := range labels {
			if c.Label == l {
				n += c.N
			}
		}
	}
	return n
}

func (d Data) TotalMentions() int { return sumCounts(d.Platforms) }

// Buyers counts mentions with purchase-adjacent intent.
func (d Data) Buyers() int { return d.intentCount("buy_signal", "recommendation_ask", "comparison") }

func (d Data) Complaints() int { return d.intentCount("complaint") }

// Empty reports a period with nothing worth sending.
func (d Data) Empty() bool {
	return d.TotalMentions() == 0 && d.RepliesPosted == 0 && d.Clicks == 0 && d.RevenueCents == 0
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// TemplateSummary is the deterministic summary used when no LLM is available.
func TemplateSummary(d Data) string {
	return fmt.Sprintf("%s, %s, %s, %s attributed. %s, %s, %s.",
		plural(d.Buyers(), "buyer", "buyers"),
		plural(d.Complaints(), "competitor complaint", "competitor complaints"),
		plural(len(d.NoisyKeywords), "noisy keyword", "noisy keywords"),
		FormatUSD(d.RevenueCents),
		plural(d.TotalMentions(), "mention", "mentions"),
		plural(d.RepliesPosted, "reply posted", "replies posted"),
		plural(d.Clicks, "click", "clicks"),
	)
}

func Subject(d Data) string {
	buyers := plural(d.Buyers(), "buyer", "buyers")
	if d.Kind == KindWeekly {
		return fmt.Sprintf("LeadEcho weekly digest: %s, %s attributed", buyers, FormatUSD(d.RevenueCents))
	}
	return fmt.Sprintf("LeadEcho daily digest: %s, %s", buyers, plural(d.TotalMentions(), "mention", "mentions"))
}

// FormatUSD renders cents as "$1,234.50" (whole dollars drop the decimals).
func FormatUSD(cents int64) string {
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	dollars := fmt.Sprint(cents / 100)
	var b strings.Builder
	for i, r := range dollars {
		if i > 0 && (len(dollars)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if rem := cents % 100; rem != 0 {
		return fmt.Sprintf("%s$%s.%02d", sign, b.String(), rem)
	}
	return sign + "$" + b.String()
}

// Facts is the plain-text fact sheet handed to the LLM for the weekly summary.
func Facts(d Data) string {
	var b strings.Builder
	fmt.Fprintf(&b, "period: %s to %s\n", d.Period.Start.Format("2006-01-02"), d.Period.End.Format("2006-01-02"))
	fmt.Fprintf(&b, "mentions: %d\nbuyers: %d\ncompetitor complaints: %d\n", d.TotalMentions(), d.Buyers(), d.Complaints())
	terms := make([]string, 0, len(d.NoisyKeywords))
	for _, k := range d.NoisyKeywords {
		terms = append(terms, k.Term)
	}
	fmt.Fprintf(&b, "noisy keywords: %s\n", strings.Join(terms, ", "))
	fmt.Fprintf(&b, "replies posted: %d\nclicks: %d\nsignups: %d\nattributed revenue: %s\n", d.RepliesPosted, d.Clicks, d.Signups, FormatUSD(d.RevenueCents))
	b.WriteString("by platform:\n")
	for _, c := range d.Platforms {
		fmt.Fprintf(&b, "  %s: %d\n", c.Label, c.N)
	}
	b.WriteString("by intent:\n")
	for _, c := range d.Intents {
		fmt.Fprintf(&b, "  %s: %d\n", c.Label, c.N)
	}
	b.WriteString("top leads:\n")
	for _, l := range d.TopLeads {
		fmt.Fprintf(&b, "  [%s, fit %.1f, %s] %s\n", l.Platform, l.Score, l.Intent, truncate(l.Title, 140))
	}
	return b.String()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
