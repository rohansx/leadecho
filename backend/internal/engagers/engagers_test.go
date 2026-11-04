package engagers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	"leadecho/internal/treg"
)

func TestParsePostRef(t *testing.T) {
	cases := []struct {
		in      string
		want    map[string]any
		wantErr bool
	}{
		{"https://www.linkedin.com/feed/update/urn:li:activity:7508596155145043968/", map[string]any{"post_urn": "urn:li:activity:7508596155145043968", "post_id": "7508596155145043968"}, false},
		{"https://www.linkedin.com/posts/jane_doe-growth-activity-7508596155145043968-AbCd?utm_source=share", map[string]any{"post_urn": "urn:li:activity:7508596155145043968", "post_id": "7508596155145043968"}, false},
		{"urn:li:ugcPost:7100000000000000000", map[string]any{"post_urn": "urn:li:ugcPost:7100000000000000000"}, false},
		{"https://linkedin.com/feed/update/urn:li:share:7200000000000000000", map[string]any{"post_urn": "urn:li:share:7200000000000000000"}, false},
		{"https://evil.example.com/urn:li:activity:1", nil, true},
		{"http://www.linkedin.com/feed/update/urn:li:activity:1", nil, true},
		{"https://www.linkedin.com/in/janedoe", nil, true},
		{"", nil, true},
	}
	for _, c := range cases {
		got, err := ParsePostRef(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("%q: err = %v", c.in, err)
			continue
		}
		if !c.wantErr && !reflect.DeepEqual(got.Body(), c.want) {
			t.Errorf("%q: body = %v, want %v", c.in, got.Body(), c.want)
		}
	}
}

const commentsBody = `{"output":{"comments":[
 {"urn":"urn:li:comment:1","text":"We need this for our SDR team","author":{"name":"Ann Lee","headline":"Head of Sales at Northwind","profileUrl":"https://www.linkedin.com/in/annlee","publicId":"annlee"},
  "replies":[{"text":"same here","author":{"name":"Bo Chen","headline":"Founder @ Tiny","profileUrl":"https://www.linkedin.com/in/bochen","publicId":"bochen"}}]},
 {"text":"","author":{"name":"No Id"}}
],"count":2}}`

const reactionsBody = `{"output":{"reactions":[
 {"reactionType":"LIKE","actor":{"name":"Ann Lee","headline":"Head of Sales at Northwind","profileUrl":"https://www.linkedin.com/in/annlee","publicId":"annlee"}},
 {"reactionType":"PRAISE","actor":{"name":"Cy Diaz","headline":"Engineer","profileUrl":"https://www.linkedin.com/in/cydiaz","publicId":"cydiaz"}}
]}}`

func TestParseEngagers(t *testing.T) {
	got := merge(parseEngagers(KindComments, []byte(commentsBody)), parseEngagers(KindReactions, []byte(reactionsBody)))
	want := []Engager{
		{Name: "Ann Lee", Headline: "Head of Sales at Northwind", ProfileURL: "https://www.linkedin.com/in/annlee", PublicID: "annlee", Kind: "comment", Comment: "We need this for our SDR team", Reaction: "LIKE", Title: "Head of Sales", Company: "Northwind"},
		{Name: "Bo Chen", Headline: "Founder @ Tiny", ProfileURL: "https://www.linkedin.com/in/bochen", PublicID: "bochen", Kind: "comment", Comment: "same here", Title: "Founder", Company: "Tiny"},
		{Name: "Cy Diaz", Headline: "Engineer", ProfileURL: "https://www.linkedin.com/in/cydiaz", PublicID: "cydiaz", Kind: "reaction", Reaction: "PRAISE", Title: "Engineer"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
}

func TestSplitHeadline(t *testing.T) {
	cases := []struct{ in, title, company string }{
		{"VP Eng at Acme | ex-Google", "VP Eng", "Acme"},
		{"CTO @ Foo Labs", "CTO", "Foo Labs"},
		{"Building things", "Building things", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		if ti, co := splitHeadline(c.in); ti != c.title || co != c.company {
			t.Errorf("%q -> %q, %q", c.in, ti, co)
		}
	}
}

type fakeTreg struct {
	mu    sync.Mutex
	tools []string
	sent  []map[string]any
}

func fakeServer(t *testing.T, f *fakeTreg) *treg.Client {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		tool := strings.TrimPrefix(r.URL.Path, "/call/")
		f.mu.Lock()
		f.tools, f.sent = append(f.tools, tool), append(f.sent, body)
		f.mu.Unlock()
		switch tool {
		case ToolComments:
			_, _ = w.Write([]byte(commentsBody))
		case ToolReactions:
			w.WriteHeader(http.StatusBadRequest) // one kind failing must not lose the other
			_, _ = w.Write([]byte(`{"error":"bad"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return treg.New(treg.Config{Token: "t", BaseURL: srv.URL}, nil, zerolog.Nop())
}

func TestFetch(t *testing.T) {
	f := &fakeTreg{}
	c := fakeServer(t, f)
	ref, _ := ParsePostRef("urn:li:activity:99")
	got, err := Fetch(context.Background(), c, "11111111-1111-1111-1111-111111111111", ref, []string{KindComments, KindReactions})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].PublicID != "annlee" {
		t.Fatalf("got %+v", got)
	}
	if strings.Join(f.tools, ",") != ToolComments+","+ToolReactions || f.sent[0]["post_urn"] != "urn:li:activity:99" || f.sent[0]["limit"] != float64(fetchLimit) {
		t.Fatalf("calls %v %v", f.tools, f.sent)
	}

	if _, err := Fetch(context.Background(), nil, "ws", ref, []string{KindComments}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("nil client err = %v", err)
	}
}

func TestValidKinds(t *testing.T) {
	cases := []struct {
		in   []string
		want []string
		ok   bool
	}{
		{nil, []string{KindComments, KindReactions}, true},
		{[]string{"reactions", "reactions"}, []string{KindReactions}, true},
		{[]string{"likes"}, nil, false},
	}
	for _, c := range cases {
		got, err := ValidKinds(c.in)
		if (err == nil) != c.ok || (c.ok && !reflect.DeepEqual(got, c.want)) {
			t.Errorf("%v -> %v %v", c.in, got, err)
		}
	}
}
