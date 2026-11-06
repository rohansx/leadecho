package handler

import (
	"time"

	"leadecho/internal/database"
)

// mentionSnippetRunes caps the post text sent with each lead; the full post
// is one click away via the inbox or the source URL.
const mentionSnippetRunes = 280

// LeadMention is what the lead actually wrote: the reason they are a lead.
type LeadMention struct {
	Title    *string    `json:"title"`
	Snippet  string     `json:"snippet"`
	URL      string     `json:"url"`
	Intent   *string    `json:"intent"`
	Score    *float32   `json:"score"`
	PostedAt *time.Time `json:"posted_at"`
}

// LeadListItem is a LeadResponse plus its source post. The extra field is
// additive, so existing API consumers are unaffected.
type LeadListItem struct {
	LeadResponse
	Mention *LeadMention `json:"mention"`
}

func leadRowToListItem(row database.ListLeadsWithMentionRow) LeadListItem {
	item := LeadListItem{LeadResponse: leadToResponse(row.Lead)}
	if !row.MentionUrl.Valid { // no linked (or a deleted) mention
		return item
	}
	m := &LeadMention{
		Snippet: truncateRunes(row.MentionContent.String, mentionSnippetRunes),
		URL:     row.MentionUrl.String,
	}
	if row.MentionTitle.Valid && row.MentionTitle.String != "" {
		m.Title = &row.MentionTitle.String
	}
	if row.MentionIntent.Valid {
		s := string(row.MentionIntent.IntentType)
		m.Intent = &s
	}
	if row.MentionScore.Valid {
		m.Score = &row.MentionScore.Float32
	}
	if row.MentionPostedAt.Valid {
		m.PostedAt = &row.MentionPostedAt.Time
	}
	item.Mention = m
	return item
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
