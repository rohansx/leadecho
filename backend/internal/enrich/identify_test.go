package enrich

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
)

func TestIdentifyXHandle(t *testing.T) {
	answers := map[string]string{
		ToolXProfile: `{"output":{"username":"janed","name":"Jane Doe","description":"VP Eng @acme. jane@acme.io"}}`,
		ToolPerson:   fullAnswers[ToolPerson],
	}
	f, c := newFake(t, answers)
	res, err := NewPipeline(c, zerolog.Nop()).Run(context.Background(), ws, Subject{Platform: "twitter", Handle: "janed"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Bio != "VP Eng @acme. jane@acme.io" || res.Steps[ToolXProfile] != StepHit {
		t.Fatalf("bio=%q steps=%v", res.Bio, res.Steps)
	}
	// The email found in the bio is what the paid person lookup matches on.
	if got := f.sent[ToolPerson]["email"]; got != "jane@acme.io" {
		t.Fatalf("person lookup sent %v", f.sent[ToolPerson])
	}
	if res.Title != "VP Engineering" || res.Company != "Acme" {
		t.Fatalf("person not applied: %+v", res)
	}
}

func TestIdentifyXKeepsNameWhenNothingToMatch(t *testing.T) {
	f, c := newFake(t, map[string]string{
		ToolXProfile: `{"output":{"name":"Sam Lee","description":"Building things"}}`,
	})
	res, err := NewPipeline(c, zerolog.Nop()).Run(context.Background(), ws, Subject{Platform: "twitter", Handle: "samlee"})
	if err != nil {
		t.Fatal(err)
	}
	if res.FullName != "Sam Lee" || res.Bio != "Building things" || !res.Found() {
		t.Fatalf("got %+v", res)
	}
	if _, called := f.sent[ToolPerson]; called {
		t.Fatal("no email or LinkedIn in the bio: the paid person lookup must not run")
	}
}

func TestIdentifyHackerNews(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user/cto1.json":
			_, _ = w.Write([]byte(`{"id":"cto1","about":"CTO at Acme.<p>https://www.linkedin.com/in/jane-doe &amp; more"}`))
		default:
			_, _ = w.Write([]byte(`null`))
		}
	}))
	defer srv.Close()
	old := hnUserURL
	hnUserURL = srv.URL + "/user/%s.json"
	defer func() { hnUserURL = old }()

	f, c := newFake(t, map[string]string{ToolPerson: fullAnswers[ToolPerson]})
	p := NewPipeline(c, zerolog.Nop())

	res, err := p.Run(context.Background(), ws, Subject{Platform: "hackernews", Handle: "cto1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Steps[StepHNProfile] != StepHit || res.Bio != "CTO at Acme.\nhttps://www.linkedin.com/in/jane-doe & more" {
		t.Fatalf("bio=%q steps=%v", res.Bio, res.Steps)
	}
	if got := f.sent[ToolPerson]["linkedin_url"]; got != "https://www.linkedin.com/in/jane-doe" {
		t.Fatalf("person lookup sent %v", f.sent[ToolPerson])
	}

	res, err = p.Run(context.Background(), ws, Subject{Platform: "hackernews", Handle: "nobody"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Steps[StepHNProfile] != StepMiss || res.Found() {
		t.Fatalf("unknown user should be a miss: %+v", res)
	}
}

func TestCleanBioStripsEscapedTags(t *testing.T) {
	if got := cleanBio("CTO &lt;script&gt;alert(1)&lt;/script&gt; at <b>Acme</b>"); got != "CTO alert(1) at Acme" {
		t.Fatalf("got %q", got)
	}
}

func TestIdentifyLeavesOtherPlatformsAlone(t *testing.T) {
	f, c := newFake(t, nil)
	res, err := NewPipeline(c, zerolog.Nop()).Run(context.Background(), ws, Subject{Platform: "reddit", Handle: "someone"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 || res.Bio != "" {
		t.Fatalf("reddit handle should make no calls, made %v", f.calls)
	}
}
