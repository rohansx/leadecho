package subrules

import (
	"regexp"
	"strings"
)

var (
	mdImageRe  = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	mdLinkRe   = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	mdBulletRe = regexp.MustCompile(`^(?:[#>*+\-\s]|\d+[.)]\s)+`)
)

const maxPageLines = 200

// RawFromPage turns a scraped rules page (old.reddit.com/r/<sub>/about/rules
// as Markdown, which carries the sidebar too) into Raw. Each line of four or
// more words becomes a rule, so quotes stay verbatim; short lines are page
// chrome ("advertise", "submit a new link") and are dropped.
func RawFromPage(markdown string) Raw {
	var raw Raw
	for _, line := range strings.Split(markdown, "\n") {
		line = mdLinkRe.ReplaceAllString(mdImageRe.ReplaceAllString(line, ""), "$1")
		line = strings.TrimSpace(mdBulletRe.ReplaceAllString(strings.TrimSpace(line), ""))
		line = strings.NewReplacer("**", "", "__", "", "`", "").Replace(line)
		if len(strings.Fields(line)) < 4 {
			continue
		}
		raw.Rules = append(raw.Rules, Rule{ShortName: clip(line, 220)})
		if len(raw.Rules) == maxPageLines {
			break
		}
	}
	return raw
}
