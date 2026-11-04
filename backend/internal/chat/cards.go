package chat

import (
	"encoding/json"
	"unicode/utf8"

	"leadecho/internal/tools"
)

const (
	maxCards       = 10
	maxCardContent = 280
)

// Cards are the mentions and leads a tool returned, rendered as cards in the UI.
type Cards struct {
	Mentions []tools.MentionView `json:"mentions,omitempty"`
	Leads    []tools.LeadView    `json:"leads,omitempty"`
}

// cardsFrom picks mentions/leads out of any tool result shape
// (search_mentions, list_leads, get_lead, weekly_summary, set_stage).
func cardsFrom(res any) *Cards {
	b, err := json.Marshal(res)
	if err != nil {
		return nil
	}
	var r struct {
		Mentions []tools.MentionView `json:"mentions"`
		Best     []tools.MentionView `json:"best_open_mentions"`
		Mention  *tools.MentionView  `json:"mention"`
		Leads    []tools.LeadView    `json:"leads"`
		Lead     *tools.LeadView     `json:"lead"`
	}
	if json.Unmarshal(b, &r) != nil {
		return nil
	}
	c := Cards{Mentions: append(r.Mentions, r.Best...), Leads: r.Leads}
	if r.Mention != nil {
		c.Mentions = append(c.Mentions, *r.Mention)
	}
	if r.Lead != nil {
		c.Leads = append(c.Leads, *r.Lead)
	}
	if len(c.Mentions) == 0 && len(c.Leads) == 0 {
		return nil
	}
	c.Mentions = c.Mentions[:min(len(c.Mentions), maxCards)]
	c.Leads = c.Leads[:min(len(c.Leads), maxCards)]
	for i := range c.Mentions {
		c.Mentions[i].Content = clip(c.Mentions[i].Content, maxCardContent)
	}
	return &c
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
