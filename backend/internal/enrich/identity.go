package enrich

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// Identity checks shared by every lookup: is this website, this company name
// and this email address the same organisation? Providers answer about
// namesakes and former employers often enough that none is taken on trust.

// CleanDomain normalises "https://www.Acme.io/about" and "WWW.acme.io" to "acme.io".
func CleanDomain(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	if i := strings.Index(d, "://"); i >= 0 {
		d = d[i+3:]
	}
	d = strings.TrimPrefix(d, "www.")
	if i := strings.IndexAny(d, "/?#:"); i >= 0 {
		d = d[:i]
	}
	return d
}

// registrable is the domain a company registers ("acme.co.uk" of
// "eng.acme.co.uk"); "" for a bare public suffix or a shared host
// ("co.uk", "github.io"), which belong to nobody.
func registrable(d string) string {
	reg, err := publicsuffix.EffectiveTLDPlusOne(CleanDomain(d))
	if err != nil {
		return ""
	}
	return reg
}

// SameDomain reports whether two websites are the same registered domain
// (subdomains and "www" ignored). Unknown values never match.
func SameDomain(a, b string) bool {
	ra := registrable(a)
	return ra != "" && ra == registrable(b)
}

func domainLabel(d string) string {
	label, _, _ := strings.Cut(registrable(d), ".")
	return label
}

// SameDomainLabel reports whether two websites share the name before the
// suffix: acme.com, acme.de and acme.co.uk are one company's country sites.
// ponytail: two unrelated firms can hold the same name under different
// suffixes, so callers that compare two companies also require the names to
// match; ask the provider for the parent company if that is not enough.
func SameDomainLabel(a, b string) bool {
	la := domainLabel(a)
	return la != "" && la == domainLabel(b)
}

// EmailDomain is the part after the @, lower-cased; "" when there is none.
func EmailDomain(email string) string {
	i := strings.LastIndex(email, "@")
	if i < 0 {
		return ""
	}
	return CleanDomain(email[i+1:])
}

var (
	// Mailbox brands that exist under many country suffixes (yahoo.co.uk).
	freeMailLabels = map[string]bool{"gmail": true, "googlemail": true, "yahoo": true, "ymail": true, "hotmail": true,
		"outlook": true, "icloud": true, "protonmail": true, "gmx": true, "yandex": true, "rediffmail": true, "tutanota": true}
	freeMailDomains = map[string]bool{"live.com": true, "msn.com": true, "me.com": true, "mac.com": true, "aol.com": true,
		"proton.me": true, "pm.me": true, "mail.com": true, "hey.com": true, "fastmail.com": true, "qq.com": true,
		"163.com": true, "126.com": true, "web.de": true, "zoho.com": true, "duck.com": true}
)

// FreeMail reports whether a domain hands mailboxes to anyone: an address
// there says nothing about where its owner works.
// ponytail: a list of the common ones; an unlisted free mailbox reads as a
// company domain and fails the agreement check instead.
func FreeMail(domain string) bool {
	reg := registrable(domain)
	return reg != "" && (freeMailDomains[reg] || freeMailLabels[domainLabel(reg)])
}

var (
	nameParenRe = regexp.MustCompile(`\([^)]*\)`)
	nameSplitRe = regexp.MustCompile(`[^a-z0-9]+`)
	legalSuffix = map[string]bool{"inc": true, "llc": true, "ltd": true, "limited": true, "corp": true, "corporation": true,
		"co": true, "company": true, "gmbh": true, "plc": true, "sa": true, "bv": true, "pvt": true, "pte": true, "pty": true,
		"ag": true, "ab": true, "oy": true, "nv": true, "sas": true, "srl": true, "spa": true, "llp": true, "lp": true}
)

// CompanyWords is a company name as comparable words: lower-cased, bracketed
// notes, punctuation and trailing legal forms dropped ("LVT (LiveView
// Technologies)" -> [lvt], "Toku S.A." -> [toku], "Acme Pte. Ltd." -> [acme]).
func CompanyWords(name string) []string {
	name = strings.ReplaceAll(nameParenRe.ReplaceAllString(strings.ToLower(name), " "), ".", "")
	words := strings.Fields(nameSplitRe.ReplaceAllString(name, " "))
	for len(words) > 1 && legalSuffix[words[len(words)-1]] {
		words = words[:len(words)-1]
	}
	return words
}

// domainSpellsName reports whether a domain is written like a company name:
// the name run together ("twobarrels.com") or its initials ("vhl.com" for
// Vista Higher Learning).
func domainSpellsName(domain, name string) bool {
	label, words := domainLabel(domain), CompanyWords(name)
	if label == "" || len(words) == 0 {
		return false
	}
	if label == strings.Join(words, "") {
		return true
	}
	initials := ""
	for _, w := range words {
		initials += w[:1]
	}
	return len(words) > 1 && label == initials
}

// EmailAtCompany reports whether an email address is at the company: on its
// website's domain (or a country variant of it), or on a domain written like
// its name. A free mailbox is never the company's.
func EmailAtCompany(email, domain, name string) bool {
	ed := EmailDomain(email)
	if ed == "" || FreeMail(ed) {
		return false
	}
	return SameDomain(ed, domain) || SameDomainLabel(ed, domain) || domainSpellsName(ed, name)
}

// checkEmployer withholds the employer when the email found for the person
// is at another organisation: one of the two lookups answered about someone
// else, and which one is unknown, so the employer is not stored as a fact.
// What disagreed is kept in IdentityMismatch.
func (r *Result) checkEmployer() {
	ed := EmailDomain(r.Email)
	if ed == "" || r.EmailStatus == "invalid" || FreeMail(ed) || (r.Company == "" && r.CompanyDomain == "") {
		return // nothing to compare
	}
	if EmailAtCompany(r.Email, r.CompanyDomain, r.Company) {
		return
	}
	r.IdentityMismatch = fmt.Sprintf("employer %q vs email domain %q", r.Company, ed)
	r.Company, r.CompanyDomain, r.CompanyLinkedInURL, r.Industry, r.Employees, r.CompanySize = "", "", "", "", "", 0
}
