package digest

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
)

// html/template escapes all mention text and neutralizes unsafe URLs.
var emailTmpl = template.Must(template.New("digest").Funcs(template.FuncMap{
	"usd":   FormatUSD,
	"trunc": truncate,
}).Parse(`<div style="font-family:system-ui,sans-serif;max-width:600px;margin:0 auto;padding:32px 24px;color:#0d1117">
  <h1 style="font-size:1.3rem;font-weight:800;margin:0 0 4px">{{.Heading}}</h1>
  <p style="color:#6a737d;font-size:0.85rem;margin:0 0 20px">{{.D.Period.Start.Format "Jan 2 15:04"}} – {{.D.Period.End.Format "Jan 2 15:04 MST"}}</p>
  {{if .D.Summary}}<p style="background:#f6f8fa;border-left:3px solid #27c17b;padding:12px 14px;line-height:1.55;margin:0 0 20px">{{.D.Summary}}</p>{{end}}
  <table style="width:100%;border-collapse:collapse;margin:0 0 20px;font-size:0.9rem"><tr>
    <td style="padding:8px;border:1px solid #e1e4e8"><strong>{{.D.TotalMentions}}</strong><br>mentions</td>
    <td style="padding:8px;border:1px solid #e1e4e8"><strong>{{.D.Buyers}}</strong><br>buyers from posts</td>
    <td style="padding:8px;border:1px solid #e1e4e8"><strong>{{.D.HiringConfirmed}}</strong><br>hiring signals{{if .D.HiringHeld}} <span style="color:#6a737d">(+{{.D.HiringHeld}} held back)</span>{{end}}</td>
    <td style="padding:8px;border:1px solid #e1e4e8"><strong>{{.D.RepliesPosted}}</strong><br>replies posted</td>
    <td style="padding:8px;border:1px solid #e1e4e8"><strong>{{.D.Clicks}}</strong><br>clicks</td>
    <td style="padding:8px;border:1px solid #e1e4e8"><strong>{{usd .D.RevenueCents}}</strong><br>attributed</td>
  </tr></table>
  {{if .D.Pipeline}}<p style="font-size:0.9rem;margin:0 0 20px"><strong>Your pipeline:</strong> {{range $i, $c := .D.Pipeline}}{{if $i}} → {{end}}{{$c.N}} {{$c.Label}}{{end}}</p>{{end}}
  <h2 style="font-size:1rem;margin:0 0 8px">Top leads</h2>
  {{range .D.TopLeads}}<div style="border-left:3px solid #27c17b;padding:6px 12px;margin:6px 0">
    <a href="{{.URL}}" style="color:#0d1117;font-weight:600">{{if .Title}}{{trunc .Title 120}}{{else}}(untitled){{end}}</a><br>
    <span style="color:#6a737d;font-size:0.8rem">{{.Platform}} · fit {{printf "%.1f" .Score}}{{if .Intent}} · {{.Intent}}{{end}}{{if .Author}} · {{.Author}}{{end}}</span>
    {{if .Who}}<br><span style="font-size:0.85rem"><strong>Who:</strong> {{.Who}}</span>{{end}}
    {{if .Draft}}<div style="background:#f6f8fa;border-radius:6px;padding:8px 10px;margin-top:6px;font-size:0.85rem;white-space:pre-wrap"><span style="color:#6a737d">Drafted message:</span><br>{{.Draft}}</div>{{end}}
  </div>{{else}}<p style="color:#6a737d">No new high-fit leads this period.</p>{{end}}
  {{if .D.Platforms}}<h2 style="font-size:1rem;margin:20px 0 8px">By platform</h2>
  <p style="font-size:0.9rem">{{range $i, $c := .D.Platforms}}{{if $i}} · {{end}}{{$c.Label}} {{$c.N}}{{end}}</p>{{end}}
  {{if .D.Intents}}<h2 style="font-size:1rem;margin:20px 0 8px">By intent</h2>
  <p style="font-size:0.9rem">{{range $i, $c := .D.Intents}}{{if $i}} · {{end}}{{$c.Label}} {{$c.N}}{{end}}</p>{{end}}
  {{if .D.NoisyKeywords}}<h2 style="font-size:1rem;margin:20px 0 8px">Noisy keywords</h2>
  <p style="font-size:0.9rem;color:#6a737d">Many mentions, low fit — consider tightening or muting:</p>
  <p style="font-size:0.9rem">{{range $i, $k := .D.NoisyKeywords}}{{if $i}} · {{end}}{{$k.Term}} ({{$k.Mentions}}, avg fit {{printf "%.1f" $k.AvgScore}}){{end}}</p>{{end}}
  {{if .D.DashboardURL}}<p style="margin-top:28px"><a href="{{.D.DashboardURL}}/inbox" style="background:#27c17b;color:#0d1117;padding:10px 20px;border-radius:6px;font-weight:700;text-decoration:none;display:inline-block">Open your inbox →</a></p>
  <p style="font-size:0.75rem;color:#999;margin-top:20px">Change digest settings under Alerts in LeadEcho.</p>{{end}}
</div>`))

func heading(k Kind) string {
	if k == KindWeekly {
		return "Weekly digest"
	}
	return "Daily digest"
}

// RenderHTML renders the email body.
func RenderHTML(d Data) (string, error) {
	var buf bytes.Buffer
	if err := emailTmpl.Execute(&buf, struct {
		Heading string
		D       Data
	}{heading(d.Kind), d}); err != nil {
		return "", fmt.Errorf("render digest: %w", err)
	}
	return buf.String(), nil
}

var slackEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// RenderSlack renders Slack mrkdwn for an incoming webhook.
func RenderSlack(d Data) string {
	var b strings.Builder
	fmt.Fprintf(&b, "*LeadEcho %s digest*\n", d.Kind)
	if d.Summary != "" {
		b.WriteString(slackEscaper.Replace(d.Summary) + "\n")
	}
	fmt.Fprintf(&b, "%d mentions · %d buyers from posts · %d hiring signals · %d replies posted · %d clicks · %s attributed\n",
		d.TotalMentions(), d.Buyers(), d.HiringConfirmed, d.RepliesPosted, d.Clicks, FormatUSD(d.RevenueCents))
	if len(d.Pipeline) > 0 {
		parts := make([]string, 0, len(d.Pipeline))
		for _, c := range d.Pipeline {
			parts = append(parts, fmt.Sprintf("%d %s", c.N, c.Label))
		}
		b.WriteString("_Pipeline:_ " + strings.Join(parts, " → ") + "\n")
	}
	for i, l := range d.TopLeads {
		if i >= 5 {
			break
		}
		title := l.Title
		if title == "" {
			title = "(untitled)"
		}
		fmt.Fprintf(&b, "• [%s, fit %.1f] <%s|%s>\n", l.Platform, l.Score, slackEscaper.Replace(l.URL), slackEscaper.Replace(truncate(title, 80)))
		if l.Who != "" {
			b.WriteString("   _" + slackEscaper.Replace(l.Who) + "_\n")
		}
		if l.Draft != "" {
			b.WriteString("   Drafted message: " + slackEscaper.Replace(truncate(strings.Join(strings.Fields(l.Draft), " "), 400)) + "\n")
		}
	}
	if len(d.NoisyKeywords) > 0 {
		terms := make([]string, 0, len(d.NoisyKeywords))
		for _, k := range d.NoisyKeywords {
			terms = append(terms, slackEscaper.Replace(k.Term))
		}
		b.WriteString("_Noisy keywords:_ " + strings.Join(terms, ", ") + "\n")
	}
	return b.String()
}
