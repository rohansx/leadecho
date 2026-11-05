package automation

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"leadecho/internal/database"
)

func triageAutomation(cfg string) database.Automation {
	return database.Automation{
		ID: aID, WorkspaceID: ws, Name: "Morning triage", Kind: KindDailyTriage,
		Frequency: "daily", RunTime: "09:30", Weekday: "monday", Timezone: "UTC",
		Enabled: true, Config: []byte(cfg),
	}
}

func newTestService(fq *fakeQ, ft *fakeTools, at time.Time) *Service {
	s := NewService(fq, ft, "", "", "https://app.test", zerolog.Nop())
	s.now = func() time.Time { return at }
	return s
}

func okTools() *fakeTools {
	return &fakeTools{handlers: map[string]func(map[string]any) (any, error){
		"search_mentions": func(map[string]any) (any, error) {
			return mentionsResult(mv("a", 8, "buy_signal", "new", now.Add(-time.Hour)))
		},
	}}
}

func TestRunDueIsIdempotentPerPeriod(t *testing.T) {
	fq := &fakeQ{automations: []database.Automation{triageAutomation(`{}`)}}
	ft := okTools()
	s := newTestService(fq, ft, now.Add(5*time.Minute))
	s.RunDue(context.Background())
	s.now = func() time.Time { return now.Add(10 * time.Minute) } // next tick, same period
	s.RunDue(context.Background())

	if len(fq.runs) != 1 || len(fq.finished) != 1 {
		t.Fatalf("runs=%d finished=%d, want 1/1", len(fq.runs), len(fq.finished))
	}
	f := fq.finished[0]
	if f.Status != "succeeded" || f.WorkspaceID != ws {
		t.Fatalf("finished = %+v", f)
	}
	var out Output
	if err := json.Unmarshal(f.Output, &out); err != nil || len(out.Items) != 1 {
		t.Fatalf("output = %s (%v)", f.Output, err)
	}
	if got := fq.lastRun[aID]; !got.Equal(now.Add(5 * time.Minute)) {
		t.Fatalf("last_run = %v", got)
	}
	if len(fq.notes) != 1 || fq.notes[0].Channel != database.NotificationChannelInApp || fq.notes[0].WorkspaceID != ws {
		t.Fatalf("notifications = %+v", fq.notes)
	}
	if !strings.Contains(fq.notes[0].Body, "post a") {
		t.Fatalf("notification body = %q", fq.notes[0].Body)
	}
}

func TestRunDueSkipsWhenNotDue(t *testing.T) {
	fq := &fakeQ{automations: []database.Automation{triageAutomation(`{}`)}}
	newTestService(fq, okTools(), now.Add(-time.Minute)).RunDue(context.Background())
	if len(fq.runs) != 0 {
		t.Fatalf("runs = %d, want 0", len(fq.runs))
	}
}

func TestRunNowRecordsFailure(t *testing.T) {
	fq := &fakeQ{}
	ft := &fakeTools{handlers: map[string]func(map[string]any) (any, error){
		"search_mentions": func(map[string]any) (any, error) { return nil, errors.New("pq: secret detail") },
	}}
	run, err := newTestService(fq, ft, now).RunNow(context.Background(), triageAutomation(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "failed" || strings.Contains(run.Error, "secret") || run.Error == "" {
		t.Fatalf("run = %+v", run)
	}
	if len(fq.notes) != 1 || !strings.Contains(fq.notes[0].Subject.String, "failed") {
		t.Fatalf("notifications = %+v", fq.notes)
	}
}

func TestRunNowEmailsWhenConfigured(t *testing.T) {
	var hits atomic.Int32
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fq := &fakeQ{}
	s := newTestService(fq, okTools(), now)
	s.mail.APIKey, s.mail.From, s.mail.Endpoint = "re_test", "LeadEcho <a@x.test>", srv.URL
	run, err := s.RunNow(context.Background(), triageAutomation(`{"email":true}`))
	if err != nil || run.Status != "succeeded" {
		t.Fatalf("run = %+v, err = %v", run, err)
	}
	if hits.Load() != 1 || !strings.Contains(body, "owner@x.test") || !strings.Contains(body, "post a") {
		t.Fatalf("hits = %d, body = %s", hits.Load(), body)
	}
	// in-app + email log
	if len(fq.notes) != 2 || fq.notes[1].Channel != database.NotificationChannelEmail {
		t.Fatalf("notifications = %+v", fq.notes)
	}

	// email off: no send
	hits.Store(0)
	if _, err := s.RunNow(context.Background(), triageAutomation(`{}`)); err != nil || hits.Load() != 0 {
		t.Fatalf("unexpected email: hits=%d err=%v", hits.Load(), err)
	}
}

func TestRenderTextEscapesNothingButHTMLDoes(t *testing.T) {
	o := Output{Summary: "s", Items: []Item{{Title: "<b>x</b>", URL: "https://x.test"}}, Suggestions: []string{"mute y"}}
	if txt := RenderText(o); !strings.Contains(txt, "<b>x</b>") || !strings.Contains(txt, "mute y") {
		t.Fatalf("text = %q", txt)
	}
	h, err := renderHTML("Name", o, "https://app.test/automations")
	if err != nil || strings.Contains(h, "<b>x</b>") || !strings.Contains(h, "&lt;b&gt;") {
		t.Fatalf("html = %q err = %v", h, err)
	}
}
