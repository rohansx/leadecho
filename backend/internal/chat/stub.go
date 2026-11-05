package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"leadecho/internal/ai"
	"leadecho/internal/tools"
)

// StubModel answers keyless (no AI provider configured) with a deterministic
// plan over the same protocol: call the read tools that fit the question,
// then summarise their results. It goes through the real agent loop.
type StubModel struct{}

const stubNote = "_No AI provider is configured, so this is a plain summary of your data. Add a key in Settings → AI for full answers._\n\n"

type stubCall struct {
	tool string
	args string
}

func stubPlan(question string) []stubCall {
	q := strings.ToLower(question)
	for _, w := range []string{"money", "revenue", "attribut", "roi", "signup"} {
		if strings.Contains(q, w) {
			return []stubCall{{"attribution_report", `{"days":30}`}}
		}
	}
	return []stubCall{{"search_mentions", `{"tier":"leads_ready","limit":5}`}, {"list_leads", `{"limit":5}`}}
}

func (StubModel) Chat(_ context.Context, _ string, msgs []ai.ChatMessage) (string, error) {
	question, results := "", []string{}
	for _, m := range msgs {
		switch {
		case m.Role != "user":
		case strings.HasPrefix(m.Content, "TOOL_RESULT "):
			results = append(results, strings.TrimPrefix(m.Content, "TOOL_RESULT "))
		case strings.HasPrefix(m.Content, "PROTOCOL_ERROR"):
		default:
			question, results = m.Content, results[:0]
		}
	}
	plan := stubPlan(question)
	if len(results) < len(plan) {
		c := plan[len(results)]
		return fmt.Sprintf(`{"tool":%q,"args":%s}`, c.tool, c.args), nil
	}
	b, err := json.Marshal(map[string]string{"answer": stubNote + stubSummary(results)})
	return string(b), err
}

type stubResult struct {
	Tool   string `json:"tool"`
	Error  string `json:"error"`
	Result struct {
		Mentions   []tools.MentionView `json:"mentions"`
		Leads      []tools.LeadView    `json:"leads"`
		ByPlatform []struct {
			Platform      string `json:"platform"`
			RepliesPosted int64  `json:"replies_posted"`
			Clicks        int64  `json:"clicks"`
			Signups       int64  `json:"signups"`
			RevenueCents  int64  `json:"revenue_cents"`
		} `json:"by_platform"`
		Totals struct {
			Revenue string `json:"revenue"`
		} `json:"totals"`
		Days int `json:"days"`
	} `json:"result"`
}

func stubSummary(results []string) string {
	var b strings.Builder
	for _, raw := range results {
		var r stubResult
		if json.Unmarshal([]byte(raw), &r) != nil {
			continue
		}
		if r.Error != "" {
			fmt.Fprintf(&b, "Could not run %s: %s\n\n", r.Tool, r.Error)
			continue
		}
		switch r.Tool {
		case "search_mentions":
			stubMentions(&b, r.Result.Mentions)
		case "list_leads":
			stubLeads(&b, r.Result.Leads)
		case "attribution_report":
			fmt.Fprintf(&b, "**Last %d days:** %s attributed revenue.\n", r.Result.Days, r.Result.Totals.Revenue)
			for _, p := range r.Result.ByPlatform {
				fmt.Fprintf(&b, "- %s: %d replies, %d clicks, %d signups, $%d.%02d\n",
					p.Platform, p.RepliesPosted, p.Clicks, p.Signups, p.RevenueCents/100, p.RevenueCents%100)
			}
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func stubMentions(b *strings.Builder, ms []tools.MentionView) {
	if len(ms) == 0 {
		b.WriteString("No high-intent mentions yet.\n\n")
		return
	}
	fmt.Fprintf(b, "**%d high-intent mentions:**\n", len(ms))
	for _, m := range ms {
		title := clip(m.Content, 80)
		if m.Title != nil && *m.Title != "" {
			title = *m.Title
		}
		score := ""
		if m.Score != nil {
			score = fmt.Sprintf(" (score %.1f)", *m.Score)
		}
		fmt.Fprintf(b, "- [%s] %s%s\n", m.Platform, title, score)
	}
	b.WriteString("\n")
}

func stubLeads(b *strings.Builder, ls []tools.LeadView) {
	if len(ls) == 0 {
		b.WriteString("No pipeline leads yet.\n\n")
		return
	}
	fmt.Fprintf(b, "**%d recent leads:**\n", len(ls))
	for _, l := range ls {
		who := "unknown"
		switch {
		case l.ContactName != nil && *l.ContactName != "":
			who = *l.ContactName
		case l.Username != nil && *l.Username != "":
			who = *l.Username
		}
		fmt.Fprintf(b, "- %s — %s\n", who, l.Stage)
	}
	b.WriteString("\n")
}
