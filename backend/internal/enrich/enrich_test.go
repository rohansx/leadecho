package enrich

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
	"time"

	"github.com/rs/zerolog"

	"leadecho/internal/database"
	"leadecho/internal/treg"
)

// fakeTreg answers /call/<tool> from a per-tool canned body and records what was sent.
type fakeTreg struct {
	mu      sync.Mutex
	answers map[string]string // tool -> JSON body (status 200)
	fail    map[string]int    // tool -> HTTP status to fail with
	sent    map[string]map[string]any
	calls   []string
	delay   time.Duration
}

func (f *fakeTreg) handler(w http.ResponseWriter, r *http.Request) {
	tool := strings.TrimPrefix(r.URL.Path, "/call/")
	time.Sleep(f.delay)
	b, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(b, &body)
	f.mu.Lock()
	f.calls = append(f.calls, tool)
	f.sent[tool] = body
	code, failing := f.fail[tool]
	ans := f.answers[tool]
	f.mu.Unlock()
	if failing {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(`{"error":"bad_input"}`))
		return
	}
	if ans == "" {
		ans = `{"output":{}}`
	}
	w.Header().Set("X-Treg-Cost-Micro", "1000")
	_, _ = w.Write([]byte(ans))
}

func newFake(t *testing.T, answers map[string]string) (*fakeTreg, *treg.Client) {
	t.Helper()
	f := &fakeTreg{answers: answers, fail: map[string]int{}, sent: map[string]map[string]any{}}
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(srv.Close)
	c := treg.New(treg.Config{Token: "tok", BaseURL: srv.URL}, nil, zerolog.Nop())
	return f, c
}

const ws = "11111111-1111-1111-1111-111111111111"

var fullAnswers = map[string]string{
	ToolPerson:      `{"output":{"full_name":"Jane Doe","title":"VP Engineering","company":"Acme","company_domain":"acme.io","linkedin_url":"https://www.linkedin.com/in/janedoe","location":"Berlin"},"_treg":{"served_by":"x"}}`,
	ToolCompany:     `{"output":{"name":"Acme Inc","domain":"acme.io","industry":"Computer Software","employees":"51-200"}}`,
	ToolEmailFind:   `{"output":{"email":"jane@acme.io","confidence":0.9}}`,
	ToolEmailVerify: `{"email":"jane@acme.io","status":"verified","confidence":98}`,
}

func TestPipelineFullChain(t *testing.T) {
	f, c := newFake(t, fullAnswers)
	res, err := NewPipeline(c, zerolog.Nop()).Run(context.Background(), ws, Subject{
		Platform: "linkedin", Handle: "janedoe", ProfileURL: "https://www.linkedin.com/in/janedoe/",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := Result{
		FullName: "Jane Doe", Title: "VP Engineering", LinkedInURL: "https://www.linkedin.com/in/janedoe", Location: "Berlin",
		Company: "Acme Inc", CompanyDomain: "acme.io", Industry: "Computer Software", Employees: "51-200", CompanySize: 51,
		Email: "jane@acme.io", EmailStatus: "verified",
	}
	got := *res
	got.Steps, got.EnrichedAt = nil, time.Time{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("result\n got %+v\nwant %+v", got, want)
	}
	for _, step := range []string{ToolPerson, ToolCompany, ToolEmailFind, ToolEmailVerify} {
		if res.Steps[step] != StepHit {
			t.Errorf("step %s = %q, want hit", step, res.Steps[step])
		}
	}
	if f.sent[ToolPerson]["linkedin_url"] != "https://www.linkedin.com/in/janedoe/" {
		t.Errorf("person body = %v", f.sent[ToolPerson])
	}
	if f.sent[ToolCompany]["domain"] != "acme.io" {
		t.Errorf("company body = %v", f.sent[ToolCompany])
	}
	if f.sent[ToolEmailFind]["linkedin_url"] != "https://www.linkedin.com/in/janedoe" {
		t.Errorf("email find body = %v", f.sent[ToolEmailFind])
	}
	if f.sent[ToolEmailVerify]["email"] != "jane@acme.io" || f.sent[ToolEmailVerify]["resolve_catchall"] != false {
		t.Errorf("verify body = %v", f.sent[ToolEmailVerify])
	}
}

func TestPipelineToleratesMissesAndErrors(t *testing.T) {
	cases := []struct {
		name      string
		subject   Subject
		answers   map[string]string
		fail      map[string]int
		wantCalls []string
		wantSteps map[string]string
		check     func(t *testing.T, r *Result, f *fakeTreg)
	}{
		{
			name:    "person miss falls back to known company name, email via domain+name",
			subject: Subject{Name: "Jane Doe", Platform: "linkedin", Handle: "janedoe", Company: "Acme"},
			answers: map[string]string{
				ToolPerson:      `{"output":{"full_name":null}}`,
				ToolCompany:     fullAnswers[ToolCompany],
				ToolEmailFind:   fullAnswers[ToolEmailFind],
				ToolEmailVerify: `{"status":"catchall"}`,
			},
			wantCalls: []string{ToolPerson, ToolCompany, ToolEmailFind, ToolEmailVerify},
			wantSteps: map[string]string{ToolPerson: StepMiss, ToolCompany: StepHit, ToolEmailFind: StepHit, ToolEmailVerify: StepHit},
			check: func(t *testing.T, r *Result, f *fakeTreg) {
				if f.sent[ToolPerson]["linkedin_handle"] != "janedoe" {
					t.Errorf("person body = %v", f.sent[ToolPerson])
				}
				if f.sent[ToolCompany]["name"] != "Acme" {
					t.Errorf("company body = %v", f.sent[ToolCompany])
				}
				if f.sent[ToolEmailFind]["domain"] != "acme.io" || f.sent[ToolEmailFind]["full_name"] != "Jane Doe" {
					t.Errorf("email body = %v", f.sent[ToolEmailFind])
				}
				if r.EmailStatus != "catchall" {
					t.Errorf("status = %q", r.EmailStatus)
				}
			},
		},
		{
			name:      "person error does not stop the chain",
			subject:   Subject{ProfileURL: "https://linkedin.com/in/jd", Company: "Acme"},
			answers:   fullAnswers,
			fail:      map[string]int{ToolPerson: http.StatusBadRequest},
			wantCalls: []string{ToolPerson, ToolCompany, ToolEmailFind, ToolEmailVerify},
			wantSteps: map[string]string{ToolPerson: StepError, ToolCompany: StepHit, ToolEmailFind: StepHit, ToolEmailVerify: StepHit},
		},
		{
			name:      "pseudonymous author: nothing to send, no paid calls",
			subject:   Subject{Platform: "reddit", Handle: "throwaway123"},
			answers:   fullAnswers,
			wantCalls: nil,
			wantSteps: map[string]string{ToolPerson: StepSkipped, ToolCompany: StepSkipped, ToolEmailFind: StepSkipped, ToolEmailVerify: StepSkipped},
		},
		{
			name:      "known email skips find but is verified",
			subject:   Subject{Email: "j@acme.io"},
			answers:   map[string]string{ToolPerson: `{"output":{"full_name":"J"}}`, ToolEmailVerify: `{"status":"invalid"}`},
			wantCalls: []string{ToolPerson, ToolCompany, ToolEmailVerify},
			wantSteps: map[string]string{ToolPerson: StepHit, ToolCompany: StepMiss, ToolEmailFind: StepSkipped, ToolEmailVerify: StepHit},
			check: func(t *testing.T, r *Result, f *fakeTreg) {
				if f.sent[ToolCompany]["email"] != "j@acme.io" {
					t.Errorf("company body = %v", f.sent[ToolCompany])
				}
				if r.Email != "j@acme.io" || r.EmailStatus != "invalid" {
					t.Errorf("email = %q %q", r.Email, r.EmailStatus)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, c := newFake(t, tc.answers)
			if tc.fail != nil {
				f.fail = tc.fail
			}
			res, err := NewPipeline(c, zerolog.Nop()).Run(context.Background(), ws, tc.subject)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(f.calls, ",") != strings.Join(tc.wantCalls, ",") {
				t.Errorf("calls = %v, want %v", f.calls, tc.wantCalls)
			}
			for k, v := range tc.wantSteps {
				if res.Steps[k] != v {
					t.Errorf("step %s = %q, want %q", k, res.Steps[k], v)
				}
			}
			if tc.check != nil {
				tc.check(t, res, f)
			}
		})
	}
}

func TestPipelineDisabled(t *testing.T) {
	if _, err := NewPipeline(nil, zerolog.Nop()).Run(context.Background(), ws, Subject{Email: "a@b.co"}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("err = %v", err)
	}
	if New(nil, nil, 10, 0, zerolog.Nop()) != nil {
		t.Fatal("New with no treg client must return nil")
	}
	var e *Enricher
	if e.Enabled() {
		t.Fatal("nil enricher must be disabled")
	}
}

type fakeCounter struct{ n int64 }

func (f fakeCounter) CountEnrichedLeadsSince(context.Context, database.CountEnrichedLeadsSinceParams) (int64, error) {
	return f.n, nil
}

type fakeBudget struct {
	mu    sync.Mutex
	calls int
	micro int64
}

func (b *fakeBudget) SetCustomerDailyBudget(_ context.Context, _ string, micro int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls++
	b.micro = micro
	return nil
}

func TestEnricherDailyCapAndBudget(t *testing.T) {
	cases := []struct {
		name      string
		used      int64
		cap       int
		wantErr   error
		wantCalls int
	}{
		{"under cap runs", 2, 3, nil, 4},
		{"at cap refuses", 3, 3, ErrDailyCap, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, c := newFake(t, fullAnswers)
			b := &fakeBudget{}
			e := New(c, fakeCounter{tc.used}, tc.cap, 50000, zerolog.Nop())
			e.budget = b
			_, err := e.Enrich(context.Background(), ws, Subject{ProfileURL: "https://linkedin.com/in/jd"})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if len(f.calls) != tc.wantCalls {
				t.Errorf("treg calls = %d, want %d", len(f.calls), tc.wantCalls)
			}
			// Budget is pushed once per workspace, before the first paid call.
			_, _ = e.Enrich(context.Background(), ws, Subject{})
			if tc.wantErr == nil && (b.calls != 1 || b.micro != 50000) {
				t.Errorf("budget calls=%d micro=%d", b.calls, b.micro)
			}
		})
	}
}

// Concurrent enrichments of one workspace must not all pass the cap check
// before any of them is stored (an engagers pull fans out ~200 at once).
func TestEnricherCapHoldsUnderConcurrency(t *testing.T) {
	f, c := newFake(t, fullAnswers)
	f.delay = 30 * time.Millisecond
	e := New(c, fakeCounter{0}, 3, 0, zerolog.Nop())
	var wg sync.WaitGroup
	var mu sync.Mutex
	ran := 0
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.Enrich(context.Background(), ws, Subject{ProfileURL: "https://linkedin.com/in/jd"}); err == nil {
				mu.Lock()
				ran++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ran > 3 {
		t.Fatalf("%d enrichments ran past a cap of 3", ran)
	}
}

func TestParseSize(t *testing.T) {
	cases := []struct {
		in   any
		want int
	}{
		{"51-200", 51},
		{"1,001-5,000", 1001},
		{float64(250), 250},
		{"10K+", 10000},
		{"about 12 employees", 12},
		{"", 0},
		{nil, 0},
		{"unknown", 0},
	}
	for _, tc := range cases {
		if got := parseSize(tc.in); got != tc.want {
			t.Errorf("parseSize(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
