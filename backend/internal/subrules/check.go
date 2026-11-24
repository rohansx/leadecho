package subrules

import (
	"regexp"
	"strings"
)

// Warning is one way a reply would likely break the subreddit's rules.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

var linkRe = regexp.MustCompile(`(?i)\bhttps?://\S+|\bwww\.\S+|\b[a-z0-9][a-z0-9-]*\.(?:com|io|ai|app|dev|co|net|org|so|sh|xyz|tech|gg|me|ly)\b(?:/\S*)?`)

// HasLink reports whether text contains a URL or a bare domain.
func HasLink(text string) bool { return linkRe.MatchString(text) }

// namedBrand returns the first brand named in text (whole word, any case).
func namedBrand(text string, brands []string) string {
	lower := strings.ToLower(text)
	for _, b := range brands {
		b = strings.TrimSpace(b)
		if len([]rune(b)) < 3 {
			continue // too short to tell from an ordinary word
		}
		re := regexp.MustCompile(`(^|[^\pL\pN])` + regexp.QuoteMeta(strings.ToLower(b)) + `($|[^\pL\pN])`)
		if re.MatchString(lower) {
			return b
		}
	}
	return ""
}

// Check compares a reply with the subreddit's summary. brands are the
// customer's product names. Deterministic: the same reply and summary
// always give the same warnings.
func Check(s *Summary, reply string, brands []string) []Warning {
	if s == nil {
		return nil
	}
	link, brand := HasLink(reply), namedBrand(reply, brands)
	if !link && brand == "" {
		return nil
	}
	sub := "r/" + s.Subreddit
	instead := "."
	if s.PromoThread != "" {
		instead = " or post it in the " + s.PromoThread + " instead."
	}
	var out []Warning
	switch {
	case !s.Available || (s.SelfPromo == PromoUnknown && s.LinksAllowed == nil):
		out = append(out, Warning{"rules_unknown", "Could not tell what " + sub + " allows on self-promotion. Check its rules before posting: many subreddits remove replies that link to or name your own product."})
	case s.SelfPromo == PromoBanned:
		if link {
			out = append(out, Warning{"link_banned", "This subreddit bans self-promotion; your reply links to your site. Remove the link" + instead})
		}
		if brand != "" {
			out = append(out, Warning{"product_banned", "This subreddit bans self-promotion; your reply names " + brand + ". Remove the product mention" + instead})
		}
	case s.SelfPromo == PromoLimited:
		if link && s.BlocksLinks() {
			out = append(out, Warning{"link_banned", "This subreddit does not allow links; your reply contains one. Remove it" + instead})
		}
		out = append(out, Warning{"promo_limited", "This subreddit limits self-promotion (see its rules below). Keep the mention brief and say you work on it" + instead})
	default: // allowed or unknown stance, links stated
		if link && s.BlocksLinks() {
			out = append(out, Warning{"link_banned", "This subreddit does not allow links; your reply contains one. Remove it."})
		}
	}
	return out
}

// PromptSection is the drafter's instruction block for the subreddit, or ""
// when nothing is known. It quotes the rules so the model reads them too.
func PromptSection(s *Summary) string {
	if s == nil || !s.Available {
		return ""
	}
	var b strings.Builder
	switch s.SelfPromo {
	case PromoBanned:
		b.WriteString("\n- This subreddit bans self-promotion. Do not name, hint at or recommend the product. Write a purely helpful reply.")
	case PromoLimited:
		b.WriteString("\n- This subreddit limits self-promotion. Prefer a purely helpful reply; mention the product at most once, briefly, with the affiliation stated, and only if it directly answers the question.")
	}
	if s.BlocksLinks() {
		b.WriteString("\n- No links of any kind.")
	}
	for _, q := range s.Quotes {
		b.WriteString("\n- Rule as written: \"" + q + "\"")
	}
	if b.Len() == 0 {
		return ""
	}
	return "\n\nSUBREDDIT RULES for r/" + s.Subreddit + " (take precedence over STYLE and VOICE; the reply must not break them):" + b.String()
}
