package enrich

import (
	"context"
	"reflect"
	"testing"

	"github.com/rs/zerolog"
)

func TestSameDomain(t *testing.T) {
	same := [][2]string{
		{"acme.io", "https://www.Acme.io/about"}, {"eng.acme.com", "acme.com"}, {"acme.co.uk", "careers.acme.co.uk"},
	}
	for _, p := range same {
		if !SameDomain(p[0], p[1]) {
			t.Errorf("%q and %q are the same site", p[0], p[1])
		}
	}
	different := [][2]string{
		{"acme.com", "acme.de"}, {"acme.co.uk", "co.uk"}, {"acme.github.io", "github.io"}, {"a.github.io", "b.github.io"},
		{"toku.example", "trytoku.example"}, {"", ""}, {"acme.com", ""},
	}
	for _, p := range different {
		if SameDomain(p[0], p[1]) {
			t.Errorf("%q and %q must not match", p[0], p[1])
		}
	}
	// Country variants share the name before the suffix.
	if !SameDomainLabel("acme.com", "acme.de") || !SameDomainLabel("www.acme.co.uk", "acme.com.br") {
		t.Error("acme.com, acme.de, acme.co.uk and acme.com.br are country variants")
	}
	if SameDomainLabel("toku.com", "trytoku.com") || SameDomainLabel("co.uk", "co.jp") || SameDomainLabel("", "") {
		t.Error("different names, bare suffixes and empty values are not variants")
	}
}

func TestEmailDomainAndFreeMail(t *testing.T) {
	if got := EmailDomain(" Jo@Mail.Acme.IO "); got != "mail.acme.io" {
		t.Errorf("EmailDomain = %q", got)
	}
	if EmailDomain("not an email") != "" || EmailDomain("") != "" {
		t.Error("no @: no domain")
	}
	for _, d := range []string{"gmail.com", "yahoo.co.uk", "outlook.com", "hotmail.fr", "proton.me", "icloud.com", "GMX.de"} {
		if !FreeMail(d) {
			t.Errorf("%s is a free mailbox", d)
		}
	}
	for _, d := range []string{"acme.io", "live.acme.com", "protonlabs.com", ""} {
		if FreeMail(d) {
			t.Errorf("%s is not a free mailbox", d)
		}
	}
}

func TestCompanyWords(t *testing.T) {
	for in, want := range map[string][]string{
		"Acme Inc.":                   {"acme"},
		"LVT (LiveView Technologies)": {"lvt"},
		"Toku S.A.":                   {"toku"},
		"Intuition Machines, Inc.":    {"intuition", "machines"},
		"Two Barrels LLC":             {"two", "barrels"},
		"Co":                          {"co"}, // never stripped to nothing
		"Acme Pte. Ltd.":              {"acme"},
	} {
		if got := CompanyWords(in); !reflect.DeepEqual(got, want) {
			t.Errorf("CompanyWords(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestEmailAtCompany(t *testing.T) {
	cases := []struct {
		email, domain, name string
		want                bool
	}{
		{"jo@acme.io", "acme.io", "Acme", true},
		{"jo@mail.acme.io", "www.acme.io", "Acme", true},
		{"jo@acme.de", "acme.com", "Acme", true},                                     // country variant
		{"jo@vhl.example", "vistahigherlearning.com", "Vista Higher Learning", true}, // the mail domain is the initials
		{"jo@acme.io", "", "Acme Inc.", true},                                        // no website known: spelt like the name
		{"jo@getacme.io", "", "Acme", false},
		{"jo@ugbazaar.example", "urbansportsclub.example", "Urban Sports Club", false},
		{"jo@ugbazaar.example", "", "Urban Sports Club", false},
		{"jo@gmail.com", "acme.io", "Acme", false}, // a free mailbox is never the company's
		{"jo@gmail.com", "", "Gmail", false},
		{"", "acme.io", "Acme", false},
		{"sk@smartcoin.co.in", "", "Olyv India (formerly SmartCoin)", true}, // renamed: the old mail domain
		{"sk@smartcoin.co.in", "", "Olyv India (formerly known as SmartCoin)", true},
		{"sk@othercoin.co.in", "", "Olyv India (formerly SmartCoin)", false},
	}
	for _, c := range cases {
		if got := EmailAtCompany(c.email, c.domain, c.name); got != c.want {
			t.Errorf("EmailAtCompany(%q, %q, %q) = %v", c.email, c.domain, c.name, got)
		}
	}
}

// A post author looked up as one company whose email is at another (the
// production case: a sports-membership company, an email at a bazaar site):
// the employer is not stored as a fact.
func TestWithholdEmployerOnEmailDisagreement(t *testing.T) {
	cases := []struct {
		name     string
		in       Result
		withheld bool
	}{
		{"email at another company", Result{FullName: "Dana Roe", Title: "Head of Growth", Company: "Urban Sports Club", CompanyDomain: "urbansportsclub.example",
			Industry: "Wellness", Employees: "501-1000", CompanySize: 501, Email: "dana@ugbazaar.example", EmailStatus: "verified"}, true},
		{"no website known, name does not fit either", Result{FullName: "Dana Roe", Company: "Urban Sports Club", Email: "dana@ugbazaar.example"}, true},
		{"email at the employer", Result{FullName: "Dana Roe", Company: "Urban Sports Club", CompanyDomain: "urbansportsclub.example", Email: "dana@urbansportsclub.example"}, false},
		{"a personal mailbox says nothing about the employer", Result{FullName: "Dana Roe", Company: "Urban Sports Club", CompanyDomain: "urbansportsclub.example", Email: "dana@gmail.com"}, false},
		{"an invalid address says nothing", Result{FullName: "Dana Roe", Company: "Urban Sports Club", CompanyDomain: "urbansportsclub.example", Email: "dana@ugbazaar.example", EmailStatus: "invalid"}, false},
		{"no email", Result{FullName: "Dana Roe", Company: "Urban Sports Club"}, false},
		{"no employer", Result{FullName: "Dana Roe", Email: "dana@ugbazaar.example"}, false},
	}
	for _, c := range cases {
		r := c.in
		r.checkEmployer()
		if got := r.IdentityMismatch != ""; got != c.withheld {
			t.Errorf("%s: mismatch = %q", c.name, r.IdentityMismatch)
			continue
		}
		if !c.withheld {
			if r.Company != c.in.Company || r.CompanyDomain != c.in.CompanyDomain {
				t.Errorf("%s: the employer must be kept, got %+v", c.name, r)
			}
			continue
		}
		if r.Company != "" || r.CompanyDomain != "" || r.Industry != "" || r.Employees != "" || r.CompanySize != 0 {
			t.Errorf("%s: the employer must not be stored as a fact, got %+v", c.name, r)
		}
		if r.FullName != c.in.FullName || r.Email != c.in.Email {
			t.Errorf("%s: the person and the email are kept, got %+v", c.name, r)
		}
		if want := `employer "Urban Sports Club" vs email domain "ugbazaar.example"`; r.IdentityMismatch != want {
			t.Errorf("%s: mismatch = %q, want %q", c.name, r.IdentityMismatch, want)
		}
	}
}

// The same through the pipeline: a post author whose bio email is at one
// company and whose lookup names another.
func TestPipelineWithholdsDisagreeingEmployer(t *testing.T) {
	_, c := newFake(t, map[string]string{
		ToolPerson:      `{"output":{"full_name":"Dana Roe","title":"Head of Growth","company":"Urban Sports Club","company_domain":"urbansportsclub.example"}}`,
		ToolCompany:     `{"output":{"name":"Urban Sports Club","domain":"urbansportsclub.example","employees":"501-1000"}}`,
		ToolEmailVerify: `{"status":"verified"}`,
	})
	res, err := NewPipeline(c, zerolog.Nop()).Run(context.Background(), ws, Subject{Name: "Dana Roe", Platform: "reddit", Email: "dana@ugbazaar.example"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Company != "" || res.Employees != "" || res.IdentityMismatch == "" || res.FullName != "Dana Roe" || res.Email != "dana@ugbazaar.example" {
		t.Fatalf("the employer must be withheld, got %+v", res)
	}
}
