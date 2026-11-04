package monitor

import "testing"

func TestCanonicalURL(t *testing.T) {
	tests := []struct{ in, want string }{
		{"https://www.reddit.com/r/saas/comments/abc/title/", "https://reddit.com/r/saas/comments/abc/title"},
		{"https://old.reddit.com/r/saas/comments/abc/title", "https://reddit.com/r/saas/comments/abc/title"},
		{"https://new.reddit.com/r/saas/comments/abc/title/?utm_source=share&utm_medium=web2x", "https://reddit.com/r/saas/comments/abc/title"},
		{"http://np.reddit.com/r/SaaS/comments/abc/title", "https://reddit.com/r/SaaS/comments/abc/title"},
		{"https://m.reddit.com/r/saas/comments/abc", "https://reddit.com/r/saas/comments/abc"},
		{"https://news.ycombinator.com/item?id=123&utm_campaign=x", "https://news.ycombinator.com/item?id=123"},
		{"https://twitter.com/jack/status/20?s=20&t=abc", "https://x.com/jack/status/20"},
		{"https://Example.com/Post?b=2&a=1&fbclid=zz#comments", "https://example.com/Post?a=1&b=2"},
		{"https://example.com/", "https://example.com"},
		{"https://dev.to/x/post?ref=hn&gclid=1", "https://dev.to/x/post"},
		{"  ", ""},
		{"not a url", "not a url"},
	}
	for _, tt := range tests {
		if got := CanonicalURL(tt.in); got != tt.want {
			t.Errorf("CanonicalURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNormalizeText(t *testing.T) {
	got := NormalizeText("Check THIS out!!  https://x.com/a?b=1 \n\n — it's   GREAT.")
	want := "check this out it s great"
	if got != want {
		t.Fatalf("NormalizeText = %q, want %q", got, want)
	}
}

func TestSimHashNearDuplicates(t *testing.T) {
	base := "Looking for a good alternative to Mailchimp for our small ecommerce store, pricing went up again and the editor is painfully slow"
	tests := []struct {
		name    string
		a, b    string
		maxDist int
		want    bool
	}{
		{"identical", base, base, nearDupBits, true},
		{"case, punctuation and urls ignored", base, "LOOKING for a good alternative to mailchimp for our small ecommerce store... pricing went up again & the editor is painfully slow https://t.co/x", nearDupBits, true},
		{"one word changed", base, "Looking for a good alternative to Mailchimp for our small ecommerce shop, pricing went up again and the editor is painfully slow", nearDupBits, true},
		{"different post", base, "We are hiring a senior Go engineer in Berlin to work on our payments platform, remote friendly with great benefits", nearDupBits, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := HammingDistance(SimHash(NormalizeText(tt.a)), SimHash(NormalizeText(tt.b)))
			if got := d <= tt.maxDist; got != tt.want {
				t.Fatalf("distance %d, within %d = %v, want %v", d, tt.maxDist, got, tt.want)
			}
		})
	}
}

func TestNearDupMaxDistance(t *testing.T) {
	tests := []struct {
		text string
		want int
	}{
		{"", -1},
		{"too short to compare", 0},
		{"this sentence has enough words in it to use the fuzzy simhash comparison", nearDupBits},
	}
	for _, tt := range tests {
		if got := nearDupMaxDistance(tt.text); got != tt.want {
			t.Errorf("nearDupMaxDistance(%q) = %d, want %d", tt.text, got, tt.want)
		}
	}
}

func TestIsBotMention(t *testing.T) {
	tests := []struct {
		author, content string
		want            bool
	}{
		{"AutoModerator", "Please read the rules", true},
		{"[deleted]", "anything", true},
		{"[removed]", "anything", true},
		{"RemindMeBot", "I will be messaging you", true},
		{"sneakpeekbot", "here's a sneak peek", true},
		{"github-actions[bot]", "deploy", true},
		{"dependabot", "bump", true},
		{"stats_bot", "x", true},
		{"some_user", "I am a bot, and this action was performed automatically. Please contact the moderators", true},
		{"jane_doe", "^^I'm ^^a ^^bot ^^beep ^^boop", true},
		{"Talbot", "Any good CRM for a small team?", false},
		{"robotics_fan", "Looking for a CRM", false},
		{"abbot", "Any good CRM?", false},
		{"", "Looking for a CRM", false},
		{"alice", "Looking for a CRM, our current one is a bottleneck", false},
	}
	for _, tt := range tests {
		if got := IsBotMention(tt.author, tt.content); got != tt.want {
			t.Errorf("IsBotMention(%q, %q) = %v, want %v", tt.author, tt.content, got, tt.want)
		}
	}
}
