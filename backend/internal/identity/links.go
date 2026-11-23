package identity

import (
	"net/url"
	"regexp"
	"strings"
)

// Link kinds. A person's identity is a set of these, each with a confidence.
const (
	KindWebsite    = "website"
	KindGitHub     = "github"
	KindX          = "x"
	KindLinkedIn   = "linkedin"
	KindBluesky    = "bluesky"
	KindMastodon   = "mastodon"
	KindYouTube    = "youtube"
	KindDevto      = "devto"
	KindHackerNews = "hackernews"
	KindReddit     = "reddit"
	KindBooking    = "booking"
	KindEmail      = "email"
)

var (
	urlRe  = regexp.MustCompile(`(?i)\bhttps?://[^\s<>"'()\[\]{}]+`)
	bareRe = regexp.MustCompile(`(?i)\b(?:www\.)?(?:github\.com|linkedin\.com/in|x\.com|twitter\.com|bsky\.app/profile|cal\.com|calendly\.com|youtube\.com|dev\.to)/[^\s<>"'()\[\]{}]+`)
	// bareDomainRe: "jane.dev" written without a scheme (common in HN bios).
	// Only common personal-site TLDs, so "node.js" or "file.txt" don't match.
	bareDomainRe = regexp.MustCompile(`(?i)(?:^|[\s(,;:])((?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+(?:com|io|dev|me|net|org|co|ai|app|xyz|sh|so|tech|blog|site|page|codes|engineer|is|cc|fm|tv|uk|de|fr|nl|ca|au|in))(?:/[^\s<>"'()\[\]{}]*)?(?:$|[\s).,;:!?])`)
	emailRe      = regexp.MustCompile(`(?i)\b[a-z0-9._%+-]+@[a-z0-9-]+(?:\.[a-z0-9-]+)*\.[a-z]{2,}\b`)
	// "name [at] domain [dot] com", common on HN profiles.
	obfuscatedEmailRe = regexp.MustCompile(`(?i)\b([a-z0-9._%+-]+)\s*(?:\[at\]|\(at\)|\s+at\s+)\s*([a-z0-9-]+(?:\s*(?:\[dot\]|\(dot\)|\.|\s+dot\s+)\s*[a-z0-9-]+)+)\b`)
	dotRe             = regexp.MustCompile(`(?i)\s*(?:\[dot\]|\(dot\)|\s+dot\s+)\s*`)
	slugRe            = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)
)

// bookingHosts are scheduling pages: a link to one is a way to book a call.
var bookingHosts = map[string]bool{
	"cal.com": true, "calendly.com": true, "savvycal.com": true, "zcal.co": true,
	"tidycal.com": true, "meetings.hubspot.com": true, "app.reclaim.ai": true,
	"koalendar.com": true, "youcanbook.me": true, "app.usemotion.com": true,
}

// notAPersonalSite are hosts that never identify one person's own website.
var notAPersonalSite = map[string]bool{
	"news.ycombinator.com": true, "reddit.com": true, "old.reddit.com": true,
	"google.com": true, "medium.com": true, "substack.com": true, "t.co": true,
	"bit.ly": true, "amazon.com": true, "apple.com": true,
	"wikipedia.org": true, "en.wikipedia.org": true, "docs.google.com": true,
	"stackoverflow.com": true, "facebook.com": true, "instagram.com": true,
	"tiktok.com": true, "discord.gg": true, "discord.com": true, "producthunt.com": true,
	"npmjs.com": true, "pypi.org": true, "crates.io": true, "gist.github.com": true,
	"keybase.io": true, "orcid.org": true, "patreon.com": true, "ko-fi.com": true,
	"buymeacoffee.com": true, "paypal.me": true, "web.archive.org": true,
	// Shared hosts where the path names the user (t.me/jane, paragraph.com/@jane):
	// the host is nobody's personal site, and its pages link to the service's
	// own accounts (Telegram's X account), not the person's.
	"t.me": true, "telegram.me": true, "telegram.org": true, "wa.me": true, "linktr.ee": true,
	"bio.link": true, "beacons.ai": true, "paragraph.com": true, "paragraph.xyz": true,
	"mirror.xyz": true, "hashnode.com": true, "gumroad.com": true, "notion.so": true,
	"threads.net": true, "drive.google.com": true,
	"dropbox.com": true, "youtu.be": true, "about.me": true, "carrd.co": true,
	"read.cv": true, "polywork.com": true, "peerlist.io": true, "huggingface.co": true,
	"kaggle.com": true, "gitlab.com": true, "codepen.io": true, "replit.com": true,
	"behance.net": true, "dribbble.com": true, "itch.io": true, "twitch.tv": true,
	"spotify.com": true, "open.spotify.com": true, "apps.apple.com": true, "play.google.com": true,
	"chrome.google.com": true, "chromewebstore.google.com": true, "vercel.com": true, "netlify.com": true,
}

// Classified is a link sorted into a kind, with the handle when the kind has one.
type Classified struct {
	Kind   string
	URL    string // normalised; "mailto:" form for emails
	Handle string // github login, x handle, linkedin slug, ... ("" for websites)
	// FromRepo: a GitHub owner taken from a repository link. Weaker than a
	// profile link: a bio can point at someone else's project.
	FromRepo bool
}

// ExtractLinks finds every URL and email address in free text (a bio, an
// "about" field, a page's text).
func ExtractLinks(text string) []Classified {
	var out []Classified
	seen := map[string]bool{}
	add := func(c Classified, ok bool) {
		if ok && !seen[c.URL] {
			seen[c.URL] = true
			out = append(out, c)
		}
	}
	for _, m := range urlRe.FindAllString(text, -1) {
		add(Classify(m))
	}
	for _, m := range bareRe.FindAllString(text, -1) {
		add(Classify("https://" + strings.TrimPrefix(strings.ToLower(m), "www.")))
	}
	for _, m := range bareDomainRe.FindAllStringSubmatch(text, -1) {
		if !strings.Contains(m[0], "@") {
			add(Classify("https://" + strings.TrimSpace(strings.Trim(m[0], " \t\n(),;:!?"))))
		}
	}
	for _, m := range emailRe.FindAllString(text, -1) {
		add(classifyEmail(m))
	}
	for _, m := range obfuscatedEmailRe.FindAllStringSubmatch(text, -1) {
		domain := dotRe.ReplaceAllString(m[2], ".")
		add(classifyEmail(m[1] + "@" + strings.ReplaceAll(domain, " ", "")))
	}
	return out
}

func classifyEmail(s string) (Classified, bool) {
	s = strings.ToLower(strings.Trim(strings.TrimPrefix(strings.TrimSpace(s), "mailto:"), ".,;"))
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	if !emailRe.MatchString(s) || strings.HasSuffix(s, ".png") || strings.HasSuffix(s, ".jpg") ||
		strings.Contains(s, "example.") || strings.HasPrefix(s, "noreply") || strings.HasPrefix(s, "no-reply") {
		return Classified{}, false
	}
	return Classified{Kind: KindEmail, URL: "mailto:" + s, Handle: s}, true
}

// Classify sorts one URL into a kind. ok is false for anything that isn't a
// person's profile, page or contact route (images, search pages, posts).
func Classify(raw string) (Classified, bool) {
	raw = strings.TrimRight(strings.TrimSpace(raw), ".,;:!?)")
	if strings.HasPrefix(strings.ToLower(raw), "mailto:") {
		return classifyEmail(raw)
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return Classified{}, false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	host = strings.TrimPrefix(host, "m.")
	parts := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
	first := ""
	if len(parts) > 0 {
		first = parts[0]
	}
	mk := func(kind, handle, canonical string) (Classified, bool) {
		if handle == "" || !slugRe.MatchString(handle) {
			return Classified{}, false
		}
		return Classified{Kind: kind, URL: canonical, Handle: handle}, true
	}
	switch {
	case host == "github.com":
		if len(parts) != 1 || reservedGitHub[strings.ToLower(first)] {
			if len(parts) >= 2 && !reservedGitHub[strings.ToLower(first)] { // a repo: its owner is the person
				c, ok := mk(KindGitHub, first, "https://github.com/"+first)
				c.FromRepo = true
				return c, ok
			}
			return Classified{}, false
		}
		return mk(KindGitHub, first, "https://github.com/"+first)
	case host == "linkedin.com" || strings.HasSuffix(host, ".linkedin.com"):
		if len(parts) >= 2 && parts[0] == "in" {
			slug := strings.ToLower(parts[1])
			return mk(KindLinkedIn, slug, "https://www.linkedin.com/in/"+slug)
		}
		return Classified{}, false
	case host == "x.com" || host == "twitter.com":
		if len(parts) != 1 || reservedX[strings.ToLower(first)] {
			return Classified{}, false
		}
		h := strings.TrimPrefix(first, "@")
		return mk(KindX, h, "https://x.com/"+h)
	case host == "bsky.app":
		if len(parts) == 2 && parts[0] == "profile" {
			return mk(KindBluesky, strings.ToLower(parts[1]), "https://bsky.app/profile/"+strings.ToLower(parts[1]))
		}
		return Classified{}, false
	case host == "youtube.com":
		if strings.HasPrefix(first, "@") && len(parts) >= 1 {
			return mk(KindYouTube, strings.TrimPrefix(first, "@"), "https://www.youtube.com/"+first)
		}
		if (first == "c" || first == "channel" || first == "user") && len(parts) >= 2 {
			return mk(KindYouTube, parts[1], "https://www.youtube.com/"+first+"/"+parts[1])
		}
		return Classified{}, false
	case host == "dev.to":
		if len(parts) == 1 {
			return mk(KindDevto, first, "https://dev.to/"+first)
		}
		return Classified{}, false
	case host == "news.ycombinator.com":
		if u.Path == "/user" {
			return mk(KindHackerNews, u.Query().Get("id"), "https://news.ycombinator.com/user?id="+u.Query().Get("id"))
		}
		return Classified{}, false
	case host == "reddit.com" || host == "old.reddit.com":
		if len(parts) >= 2 && (parts[0] == "user" || parts[0] == "u") {
			return mk(KindReddit, parts[1], "https://www.reddit.com/user/"+parts[1])
		}
		return Classified{}, false
	case bookingHosts[host] || strings.HasSuffix(host, ".cal.com"):
		if len(parts) == 0 {
			return Classified{}, false
		}
		return Classified{Kind: KindBooking, URL: "https://" + host + "/" + strings.Join(parts, "/")}, true
	case mastodonPath(host, first):
		if c, ok := mk(KindMastodon, strings.TrimPrefix(first, "@"), "https://"+host+"/"+first); ok {
			c.Handle += "@" + host
			return c, true
		}
		return Classified{}, false
	case notAPersonalSite[host] || isAssetPath(u.Path):
		return Classified{}, false
	}
	// A personal site: keep only the origin, so "/blog/post-1" and "/" are one site.
	if u.Port() != "" { // rare on the web; keeps a non-default port reachable
		return Classified{Kind: KindWebsite, URL: u.Scheme + "://" + strings.ToLower(u.Host)}, true
	}
	return Classified{Kind: KindWebsite, URL: "https://" + host}, true
}

func mastodonPath(host, first string) bool {
	return strings.HasPrefix(first, "@") && (strings.Contains(host, "mastodon") || mastodonHosts[host])
}

var mastodonHosts = map[string]bool{"hachyderm.io": true, "fosstodon.org": true, "infosec.exchange": true, "mas.to": true, "mstdn.social": true, "techhub.social": true, "ruby.social": true, "sigmoid.social": true}

func isAssetPath(p string) bool {
	p = strings.ToLower(p)
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".pdf", ".zip", ".css", ".js", ".ico", ".mp4"} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

var reservedGitHub = map[string]bool{
	"about": true, "features": true, "pricing": true, "orgs": true, "sponsors": true, "topics": true,
	"collections": true, "trending": true, "marketplace": true, "login": true, "join": true, "settings": true,
	"apps": true, "search": true, "explore": true, "issues": true, "pulls": true, "notifications": true,
	"site": true, "security": true, "enterprise": true, "readme": true, "events": true, "github": false,
}

var reservedX = map[string]bool{
	"home": true, "search": true, "intent": true, "share": true, "hashtag": true, "i": true,
	"explore": true, "settings": true, "login": true, "signup": true, "tos": true, "privacy": true,
}
