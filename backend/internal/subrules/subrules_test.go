package subrules

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSummarize(t *testing.T) {
	no := false
	tests := []struct {
		name      string
		raw       Raw
		promo     string
		links     *bool
		karma     int
		age       string
		thread    string
		minQuotes int
	}{
		{
			name:  "nothing about promotion",
			raw:   Raw{Rules: []Rule{{ShortName: "Be civil", Description: "No personal attacks."}}},
			promo: PromoUnknown,
		},
		{
			name:      "banned outright",
			raw:       Raw{Rules: []Rule{{ShortName: "No self-promotion", Description: "Posts or comments advertising your product will be removed."}}},
			promo:     PromoBanned,
			minQuotes: 1,
		},
		{
			name: "banned except the weekly thread",
			raw: Raw{
				Rules:   []Rule{{ShortName: "No advertising", Description: "Do not promote your own product."}},
				Sidebar: "Share your startup in the Weekly Feedback Thread every Monday.",
			},
			promo:     PromoLimited,
			thread:    "Weekly Feedback Thread",
			minQuotes: 1,
		},
		{
			name:      "10 percent rule",
			raw:       Raw{Rules: []Rule{{ShortName: "Self-promotion", Description: "Follow the 10% rule: most of your activity must not be about your own product."}}},
			promo:     PromoLimited,
			minQuotes: 1,
		},
		{
			name:  "allowed",
			raw:   Raw{Sidebar: "Self-promotion is welcome here, show us what you built!"},
			promo: PromoAllowed,
		},
		{
			name:      "no links",
			raw:       Raw{Rules: []Rule{{ShortName: "No links", Description: "Links to external sites are not allowed in comments."}}},
			promo:     PromoUnknown,
			links:     &no,
			minQuotes: 1,
		},
		{
			name: "karma and account age",
			raw: Raw{Rules: []Rule{{
				ShortName:   "Account requirements",
				Description: "Your account must be at least 30 days old and have 100 comment karma to post.",
			}}},
			promo:     PromoUnknown,
			karma:     100,
			age:       "30 days",
			minQuotes: 1,
		},
		{
			name:   "promo saturday",
			raw:    Raw{Sidebar: "Self-Promotion Saturday: plug your project in the pinned post."},
			promo:  PromoLimited,
			thread: "Self-Promotion Saturday",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Summarize("SaaS", tt.raw)
			if !s.Available || s.Subreddit != "SaaS" || s.RulesURL != "https://www.reddit.com/r/SaaS/about/rules" {
				t.Fatalf("header wrong: %+v", s)
			}
			if s.SelfPromo != tt.promo {
				t.Errorf("self_promo = %q, want %q", s.SelfPromo, tt.promo)
			}
			if (s.LinksAllowed == nil) != (tt.links == nil) || (s.LinksAllowed != nil && *s.LinksAllowed != *tt.links) {
				t.Errorf("links_allowed = %v, want %v", s.LinksAllowed, tt.links)
			}
			if s.MinKarma != tt.karma {
				t.Errorf("min_karma = %d, want %d", s.MinKarma, tt.karma)
			}
			if s.MinAccountAge != tt.age {
				t.Errorf("min_account_age = %q, want %q", s.MinAccountAge, tt.age)
			}
			if !strings.EqualFold(s.PromoThread, tt.thread) {
				t.Errorf("promo_thread = %q, want %q", s.PromoThread, tt.thread)
			}
			if len(s.Quotes) < tt.minQuotes || len(s.Quotes) > 3 {
				t.Errorf("quotes = %v, want at least %d (max 3)", s.Quotes, tt.minQuotes)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	no := false
	banned := &Summary{Subreddit: "SaaS", Available: true, SelfPromo: PromoBanned}
	bannedThread := &Summary{Subreddit: "SaaS", Available: true, SelfPromo: PromoBanned, PromoThread: "weekly promo thread"}
	limited := &Summary{Subreddit: "SaaS", Available: true, SelfPromo: PromoLimited}
	noLinks := &Summary{Subreddit: "SaaS", Available: true, SelfPromo: PromoAllowed, LinksAllowed: &no}
	allowed := &Summary{Subreddit: "SaaS", Available: true, SelfPromo: PromoAllowed}
	down := &Summary{Subreddit: "SaaS", SelfPromo: PromoUnknown}
	brands := []string{"LeadEcho", "ab"}

	tests := []struct {
		name  string
		s     *Summary
		reply string
		want  []string // warning codes
		in    string   // substring expected in the first message
	}{
		{"no summary", nil, "see https://x.com", nil, ""},
		{"clean reply", banned, "Try talking to five customers first.", nil, ""},
		{"link in banned sub", banned, "Check https://example.com/start", []string{"link_banned"}, "Remove the link."},
		{"bare domain counts", banned, "see leadecho.app for more", []string{"link_banned", "product_banned"}, "links to your site"},
		{"brand in banned sub", banned, "I work on LeadEcho, it does this.", []string{"product_banned"}, "names LeadEcho"},
		{"brand any case", banned, "i work on leadecho.", []string{"product_banned"}, ""},
		{"brand inside a word is not a mention", banned, "the leadechoes of history", nil, ""},
		{"short brand ignored", banned, "ab testing helps", nil, ""},
		{"thread suggested", bannedThread, "LeadEcho does this", []string{"product_banned"}, "weekly promo thread"},
		{"limited", limited, "I work on LeadEcho, which does this.", []string{"promo_limited"}, "limits self-promotion"},
		{"links banned only", noLinks, "see https://x.com", []string{"link_banned"}, "does not allow links"},
		{"allowed brand", allowed, "I work on LeadEcho.", nil, ""},
		{"rules unreadable", down, "I work on LeadEcho.", []string{"rules_unknown"}, "Check its rules"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Check(tt.s, tt.reply, brands)
			var codes []string
			for _, w := range got {
				codes = append(codes, w.Code)
			}
			if strings.Join(codes, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("codes = %v, want %v", codes, tt.want)
			}
			if tt.in != "" && !strings.Contains(got[0].Message, tt.in) {
				t.Errorf("message %q lacks %q", got[0].Message, tt.in)
			}
		})
	}
}

func TestPromptSection(t *testing.T) {
	if PromptSection(nil) != "" || PromptSection(&Summary{Subreddit: "x"}) != "" {
		t.Fatal("unknown rules must add nothing to the prompt")
	}
	if p := PromptSection(&Summary{Subreddit: "x", Available: true, SelfPromo: PromoUnknown}); p != "" {
		t.Fatalf("nothing known, got %q", p)
	}
	p := PromptSection(&Summary{Subreddit: "SaaS", Available: true, SelfPromo: PromoBanned, Quotes: []string{"No self-promotion"}})
	for _, want := range []string{"r/SaaS", "bans self-promotion", "Do not name", "No links", `"No self-promotion"`} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q: %s", want, p)
		}
	}
}

type memCache struct {
	m   map[string][]byte
	ttl map[string]time.Duration
}

func (c *memCache) Get(_ context.Context, k string) ([]byte, bool) { b, ok := c.m[k]; return b, ok }
func (c *memCache) Set(_ context.Context, k string, v []byte, ttl time.Duration) {
	c.m[k], c.ttl[k] = v, ttl
}

func TestServiceGet(t *testing.T) {
	cache := &memCache{m: map[string][]byte{}, ttl: map[string]time.Duration{}}
	calls := 0
	fail := false
	svc := New(func(_ context.Context, sub string) (Raw, error) {
		calls++
		if fail {
			return Raw{}, errors.New("403")
		}
		return Raw{Rules: []Rule{{ShortName: "No self-promotion"}}}, nil
	}, cache)
	ctx := context.Background()

	if svc.Get(ctx, "bad name!") != nil || (*Service)(nil).Get(ctx, "SaaS") != nil {
		t.Fatal("invalid name or nil service must give nil")
	}
	s := svc.Get(ctx, "r/SaaS")
	if s == nil || s.SelfPromo != PromoBanned || s.Subreddit != "SaaS" {
		t.Fatalf("got %+v", s)
	}
	svc.Get(ctx, "saas") // cached, case-insensitive
	if calls != 1 || cache.ttl["subrules:v1:saas"] != cacheTTL {
		t.Fatalf("calls = %d, ttl = %v", calls, cache.ttl)
	}

	fail = true
	s = svc.Get(ctx, "startups")
	if s == nil || s.Available || s.SelfPromo != PromoUnknown {
		t.Fatalf("failed fetch should be unavailable, got %+v", s)
	}
	if cache.ttl["subrules:v1:startups"] != failTTL {
		t.Fatalf("failure ttl = %v", cache.ttl["subrules:v1:startups"])
	}
	svc.Get(ctx, "startups")
	if calls != 2 {
		t.Fatalf("a cached failure must not refetch, calls = %d", calls)
	}
}

func TestSubredditOf(t *testing.T) {
	tests := []struct{ url, meta, want string }{
		{"https://reddit.com/r/SaaS/comments/abc/x/", `{"subreddit":"SaaS"}`, "SaaS"},
		{"https://reddit.com/r/startups/comments/abc/x/", `{}`, "startups"},
		{"https://reddit.com/r/startups", ``, "startups"},
		{"https://reddit.com/user/x", `{}`, ""},
		{"https://reddit.com/x", `{"subreddit":"r/indiehackers"}`, "indiehackers"},
	}
	for _, tt := range tests {
		if got := SubredditOf(tt.url, []byte(tt.meta)); got != tt.want {
			t.Errorf("SubredditOf(%q, %s) = %q, want %q", tt.url, tt.meta, got, tt.want)
		}
	}
}
