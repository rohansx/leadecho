package ai

import (
	"regexp"
	"strings"
	"unicode"
)

// Which language a job posting is written in, so the message to the employer
// can be written in it. Only the title is stored, so only the title is read.
//
// ponytail: Spanish and Portuguese by word list, the two that reached
// production as English emails. Anything unclear stays English. Add a list
// per language when another one shows up.

var (
	// Words a job title only uses in that language.
	spanishWords    = wordSet("desarrollador desarrolladora desarrollo ingeniero ingeniera trabajo programación programacion jefe jefa responsable")
	portugueseWords = wordSet("desenvolvedor desenvolvedora desenvolvimento engenheiro engenheira vaga pessoa pleno sênior programação programacao")
	// Written the same in both: they only say "not English".
	iberianWords = wordSet("analista programador programadora de para remoto remota sistemas técnico tecnico líder")

	spanishEndings    = []string{"ción", "cion", "idad"}
	portugueseEndings = []string{"ção", "ções", "idade"}

	spanishPlaces    = regexp.MustCompile(`(?i)\b(spain|españa|espana|mexico|méxico|chile|argentina|colombia|per[uú]|uruguay|ecuador|costa rica|paraguay|bolivia|venezuela|guatemala|panam[aá])`)
	portuguesePlaces = regexp.MustCompile(`(?i)\b(brazil|brasil|portugal)`)
)

func wordSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(s) {
		out[w] = true
	}
	return out
}

// PostingLanguage returns "Spanish" or "Portuguese" when the posting's title
// is clearly written in that language, and "" (write in English) otherwise.
// The location only settles a title that is not English but reads the same
// in both: an English title posted in Chile stays English.
func PostingLanguage(title, location string) string {
	es, pt, either := 0, 0, 0
	for _, w := range strings.FieldsFunc(strings.ToLower(title), func(r rune) bool { return !unicode.IsLetter(r) }) {
		switch {
		case spanishWords[w] || hasEnding(w, spanishEndings):
			es++
		case portugueseWords[w] || hasEnding(w, portugueseEndings):
			pt++
		case iberianWords[w]:
			either++
		}
	}
	switch {
	case es > 0 && pt == 0:
		return "Spanish"
	case pt > 0 && es == 0:
		return "Portuguese"
	case es == 0 && pt == 0 && either >= 2:
		if portuguesePlaces.MatchString(location) {
			return "Portuguese"
		}
		if spanishPlaces.MatchString(location) {
			return "Spanish"
		}
	}
	return ""
}

func hasEnding(w string, endings []string) bool {
	for _, e := range endings {
		if len(w) > len(e)+2 && strings.HasSuffix(w, e) {
			return true
		}
	}
	return false
}
