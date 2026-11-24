// Package subrules turns a subreddit's own rules and sidebar into a short
// summary of what it allows (self-promotion, links, karma and account age,
// a promo thread), so a drafted reply can follow them and the customer is
// warned before posting one that would break them.
//
// Extraction is deterministic: every value comes from text the subreddit
// wrote, quotes are verbatim, and anything not stated stays unknown.
// ponytail: keyword heuristics, not an LLM. Upgrade to a cheap-tier LLM pass
// over Raw if the heuristics misread too many subreddits.
package subrules

import (
	"regexp"
	"strconv"
	"strings"
)

// Rule is one entry of /r/<sub>/about/rules (Reddit's own field names).
type Rule struct {
	ShortName   string `json:"short_name"`
	Description string `json:"description"`
}

// Raw is what Reddit says about a subreddit: its rules and its sidebar
// (description, public description and submit text joined). The source of
// truth; Summary is derived from it on every read.
type Raw struct {
	Rules   []Rule `json:"rules"`
	Sidebar string `json:"sidebar"`
}

// Self-promotion stances.
const (
	PromoAllowed = "allowed"
	PromoLimited = "limited"
	PromoBanned  = "banned"
	PromoUnknown = "unknown"
)

// Summary is the compact view shown next to a draft and fed to the drafter.
type Summary struct {
	Subreddit string `json:"subreddit"`
	// Available is false when Reddit could not be read: everything else is unknown.
	Available     bool     `json:"available"`
	SelfPromo     string   `json:"self_promo"`
	LinksAllowed  *bool    `json:"links_allowed"` // nil: not stated
	MinKarma      int      `json:"min_karma,omitempty"`
	MinAccountAge string   `json:"min_account_age,omitempty"` // as written, e.g. "30 days"
	PromoThread   string   `json:"promo_thread,omitempty"`
	Quotes        []string `json:"quotes"`
	RulesURL      string   `json:"rules_url"`
}

// BlocksLinks reports whether a link in a reply would break the rules.
func (s *Summary) BlocksLinks() bool {
	return s != nil && (s.SelfPromo == PromoBanned || (s.LinksAllowed != nil && !*s.LinksAllowed))
}

func rulesURL(sub string) string { return "https://www.reddit.com/r/" + sub + "/about/rules" }

// Unavailable is the summary for a subreddit whose rules could not be read.
func Unavailable(sub string) Summary {
	return Summary{Subreddit: sub, SelfPromo: PromoUnknown, Quotes: []string{}, RulesURL: rulesURL(sub)}
}

var (
	promoRe    = regexp.MustCompile(`self[- ]?promo|advertis|promot|shill|plug(ging)? (your|own)`)
	negationRe = regexp.MustCompile(`\b(no|not|never|don'?t|do not|prohibited|banned|forbidden|disallowed|isn'?t allowed|will be removed)\b`)
	limitedRe  = regexp.MustCompile(`10 ?%|9:1|\bratio\b|\bonly (in|on|within|allowed)\b|\b(weekly|monthly|daily)\b|\bthread\b|\b(mondays?|tuesdays?|wednesdays?|thursdays?|fridays?|saturdays?|sundays?)\b|\blimit|\bminimum\b|\bsparingly\b|\boccasional|\bonce (a|per|every)\b|\bcontribut|\bwith (mod|moderator) (approval|permission)\b`)
	allowedRe  = regexp.MustCompile(`self[- ]?promo\w* (is )?(allowed|welcome|ok|okay|permitted|fine)|feel free to (share|promote|post your)`)
	linkRuleRe = regexp.MustCompile(`\b(links?|urls?|linking)\b`)
	karmaRe    = regexp.MustCompile(`(\d[\d,]*)\+? ?(?:combined |comment |post |link |total )?karma|karma (?:of |above |over |at least |minimum (?:of )?|requirement (?:of )?|>=? ?)*(\d[\d,]*)`)
	ageRe      = regexp.MustCompile(`(\d+) ?(day|week|month|year)s?`)
	threadRe   = regexp.MustCompile(`(?i)\b(?:weekly|monthly|daily|bi-?weekly)\s+(?:["'“]?[\w'’-]+["'”]?\s+){0,4}?(?:thread|megathread|post)\b|\b(?:self[- ]?promo\w*|showoff|show-off|promo|feedback|share your \w+)\s+(?:monday|tuesday|wednesday|thursday|friday|saturday|sunday)s?\b`)
	threadKind = regexp.MustCompile(`(?i)promo|share|show|feedback|plug|launch|advertis|project|startup|app`)
	sentenceRe = regexp.MustCompile(`[^.!?\n]+`)
)

// Summarize derives the summary from what the subreddit wrote.
func Summarize(sub string, raw Raw) Summary {
	s := Unavailable(sub)
	s.Available = true
	all := strings.ToLower(raw.Sidebar)
	var promoRules []string
	for _, sent := range sentenceRe.FindAllString(all, -1) {
		if promoRe.MatchString(sent) {
			promoRules = append(promoRules, sent)
		}
	}
	for _, r := range raw.Rules {
		text := strings.ToLower(r.ShortName + ". " + r.Description)
		all += "\n" + text
		quote := false
		if promoRe.MatchString(text) {
			promoRules = append(promoRules, text)
			quote = true
		}
		if linkRuleRe.MatchString(text) && negationRe.MatchString(text) && !limitedRe.MatchString(text) {
			no := false
			s.LinksAllowed = &no
			quote = true
		}
		if karmaRe.MatchString(text) || (strings.Contains(text, "account") && ageRe.MatchString(text)) {
			quote = true
		}
		if quote && len(s.Quotes) < 3 {
			s.Quotes = append(s.Quotes, quoteOf(r))
		}
	}
	s.SelfPromo = promoStance(promoRules)
	s.MinKarma = minKarma(all)
	s.MinAccountAge = minAccountAge(all)
	s.PromoThread = promoThread(raw)
	if s.PromoThread != "" && s.SelfPromo == PromoBanned {
		s.SelfPromo = PromoLimited // banned outside its own thread
	}
	return s
}

// promoStance: the strictest stance any promotion rule takes.
func promoStance(promoRules []string) string {
	stance := PromoUnknown
	for _, t := range promoRules {
		switch {
		case limitedRe.MatchString(t):
			if stance != PromoBanned {
				stance = PromoLimited
			}
		case negationRe.MatchString(t):
			stance = PromoBanned
		case allowedRe.MatchString(t):
			if stance == PromoUnknown {
				stance = PromoAllowed
			}
		default:
			// A rule about promotion that says neither: treat as a limit.
			if stance == PromoUnknown {
				stance = PromoLimited
			}
		}
	}
	return stance
}

func quoteOf(r Rule) string {
	q := strings.TrimSpace(r.ShortName)
	if d := clip(r.Description, 220); d != "" {
		if q != "" {
			q += ": "
		}
		q += d
	}
	return q
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func minKarma(text string) int {
	best := 0
	for _, m := range karmaRe.FindAllStringSubmatch(text, -1) {
		n := m[1]
		if n == "" {
			n = m[2]
		}
		v, err := strconv.Atoi(strings.ReplaceAll(n, ",", ""))
		if err == nil && v > best && v < 1_000_000 {
			best = v
		}
	}
	return best
}

// minAccountAge is the first age stated in a sentence about accounts.
func minAccountAge(text string) string {
	for _, sent := range sentenceRe.FindAllString(text, -1) {
		if !strings.Contains(sent, "account") {
			continue
		}
		if m := ageRe.FindStringSubmatch(sent); m != nil {
			unit := m[2]
			if m[1] != "1" {
				unit += "s"
			}
			return m[1] + " " + unit
		}
	}
	return ""
}

func promoThread(raw Raw) string {
	texts := []string{raw.Sidebar}
	for _, r := range raw.Rules {
		texts = append(texts, r.ShortName+". "+r.Description)
	}
	for _, t := range texts {
		for _, m := range threadRe.FindAllString(t, -1) {
			if threadKind.MatchString(m) {
				return clip(m, 80)
			}
		}
	}
	return ""
}
