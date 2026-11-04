package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"leadecho/internal/crmsync"
)

var allScopes = []string{"read", "write"}

func newTestRegistry() (*Registry, *fakeQ, *fakeCRM, *fakeMutes) {
	q, crm, mutes := &fakeQ{}, &fakeCRM{}, &fakeMutes{}
	return New(Deps{Q: q, Drafter: fakeDrafter{}, CRM: crm, Mutes: mutes}), q, crm, mutes
}

func call(t *testing.T, r *Registry, name, args string) (map[string]any, error) {
	t.Helper()
	out, err := r.Call(context.Background(), ws, allScopes, name, json.RawMessage(args))
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("result not JSON-encodable: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("result must be a JSON object: %s", b)
	}
	return m, nil
}

func wantUserErr(t *testing.T, err error, contains string) {
	t.Helper()
	var ue *Error
	if !errors.As(err, &ue) {
		t.Fatalf("want *tools.Error containing %q, got %v", contains, err)
	}
	if !strings.Contains(ue.Error(), contains) {
		t.Fatalf("error %q does not contain %q", ue.Error(), contains)
	}
}

func TestRegistryCatalog(t *testing.T) {
	r, _, _, _ := newTestRegistry()
	want := []string{"search_mentions", "list_leads", "get_lead", "create_listener", "tune_listener", "mute",
		"draft_reply", "queue_reply", "push_to_crm", "set_stage", "attribution_report", "weekly_summary"}
	got := map[string]Tool{}
	for _, tl := range r.List() {
		got[tl.Name] = tl
	}
	if len(got) != len(want) {
		t.Fatalf("got %d tools, want %d", len(got), len(want))
	}
	for _, name := range want {
		tl, ok := got[name]
		if !ok {
			t.Fatalf("missing tool %s", name)
		}
		if tl.Description == "" || (tl.Scope != "read" && tl.Scope != "write") || tl.Handler == nil {
			t.Fatalf("tool %s incomplete: %+v", name, tl)
		}
		var schema struct {
			Type       string         `json:"type"`
			Properties map[string]any `json:"properties"`
			Additional *bool          `json:"additionalProperties"`
		}
		if err := json.Unmarshal(tl.InputSchema, &schema); err != nil || schema.Type != "object" || schema.Properties == nil {
			t.Fatalf("tool %s: bad schema %s (%v)", name, tl.InputSchema, err)
		}
		if schema.Additional == nil || *schema.Additional {
			t.Fatalf("tool %s: schema must set additionalProperties:false", name)
		}
	}
}

func TestCallDispatchErrors(t *testing.T) {
	r, _, _, _ := newTestRegistry()
	ctx := context.Background()
	if _, err := r.Call(ctx, ws, allScopes, "nope", nil); !errors.Is(err, ErrUnknownTool) {
		t.Fatalf("want ErrUnknownTool, got %v", err)
	}
	_, err := r.Call(ctx, ws, []string{"read"}, "set_stage", json.RawMessage(`{"lead_id":"`+leadID+`","stage":"qualified"}`))
	wantUserErr(t, err, "write scope")
	_, err = r.Call(ctx, "", allScopes, "list_leads", nil)
	wantUserErr(t, err, "workspace")
	// Unknown fields and wrong types are rejected, not silently ignored.
	_, err = r.Call(ctx, ws, allScopes, "list_leads", json.RawMessage(`{"stagee":"qualified"}`))
	wantUserErr(t, err, "invalid arguments")
	_, err = r.Call(ctx, ws, allScopes, "list_leads", json.RawMessage(`{"limit":"ten"}`))
	wantUserErr(t, err, "invalid arguments")
	_, err = r.Call(ctx, ws, allScopes, "list_leads", json.RawMessage(`[1]`))
	wantUserErr(t, err, "invalid arguments")
	// null / empty args mean "no arguments".
	if _, err := r.Call(ctx, ws, allScopes, "list_leads", json.RawMessage(`null`)); err != nil {
		t.Fatal(err)
	}
}

func TestSearchMentions(t *testing.T) {
	tests := []struct {
		name, args, wantCall, wantErr string
		wantLimit                     int32
	}{
		{"default recent", `{}`, "ListMentions", "", defaultLimit},
		{"full text", `{"query":"  crm alternative "}`, "SearchMentions", "", defaultLimit},
		{"status", `{"status":"new","limit":5,"offset":10}`, "ListMentionsByStatus", "", 5},
		{"intent", `{"intent":"buy_signal"}`, "ListMentionsByIntent", "", defaultLimit},
		{"platform", `{"platform":"reddit"}`, "ListMentionsByPlatform", "", defaultLimit},
		{"leads ready", `{"tier":"leads_ready"}`, "ListMentionsLeadsReady", "", defaultLimit},
		{"limit clamped", `{"limit":5000}`, "ListMentions", "", maxLimit},
		{"bad status", `{"status":"deleted"}`, "", "status", 0},
		{"bad intent", `{"intent":"love"}`, "", "intent", 0},
		{"bad platform", `{"platform":"myspace"}`, "", "platform", 0},
		{"bad tier", `{"tier":"gold"}`, "", "tier", 0},
		{"two filters", `{"status":"new","intent":"buy_signal"}`, "", "at most one", 0},
		{"negative offset", `{"offset":-1}`, "", "offset", 0},
		{"query too long", `{"query":"` + strings.Repeat("x", 501) + `"}`, "", "query", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, q, _, _ := newTestRegistry()
			out, err := call(t, r, "search_mentions", tt.args)
			if tt.wantErr != "" {
				wantUserErr(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if q.lastCall != tt.wantCall || q.lastLimit != tt.wantLimit {
				t.Fatalf("called %s limit %d, want %s limit %d", q.lastCall, q.lastLimit, tt.wantCall, tt.wantLimit)
			}
			items := out["mentions"].([]any)
			m := items[0].(map[string]any)
			if m["id"] != mentID || m["intent"] != "buy_signal" || m["author"] != "alice" {
				t.Fatalf("bad mention view %v", m)
			}
		})
	}
	r, q, _, _ := newTestRegistry()
	call(t, r, "search_mentions", `{"query":"  crm  "}`)
	if q.lastQuery != "crm" {
		t.Fatalf("query not trimmed: %q", q.lastQuery)
	}
}

func TestListLeads(t *testing.T) {
	r, q, _, _ := newTestRegistry()
	if _, err := call(t, r, "list_leads", `{"stage":"qualified","limit":3}`); err != nil {
		t.Fatal(err)
	}
	if q.lastCall != "ListLeadsByStage:qualified" || q.lastLimit != 3 {
		t.Fatalf("got %s %d", q.lastCall, q.lastLimit)
	}
	out, _ := call(t, r, "list_leads", `{}`)
	if q.lastCall != "ListLeads" || len(out["leads"].([]any)) != 1 {
		t.Fatalf("got %s %v", q.lastCall, out)
	}
	_, err := call(t, r, "list_leads", `{"stage":"won"}`)
	wantUserErr(t, err, "stage")
}

func TestGetLead(t *testing.T) {
	r, _, _, _ := newTestRegistry()
	out, err := call(t, r, "get_lead", `{"lead_id":"`+leadID+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	lead := out["lead"].(map[string]any)
	if lead["id"] != leadID || lead["mention_id"] != mentID {
		t.Fatalf("bad lead %v", lead)
	}
	if out["mention"].(map[string]any)["id"] != mentID || len(out["history"].([]any)) != 1 {
		t.Fatalf("missing mention/history: %v", out)
	}
	_, err = call(t, r, "get_lead", `{"lead_id":"`+missing+`"}`)
	wantUserErr(t, err, "not found")
	_, err = call(t, r, "get_lead", `{"lead_id":"nope"}`)
	wantUserErr(t, err, "lead_id")
	_, err = call(t, r, "get_lead", `{}`)
	wantUserErr(t, err, "lead_id")
	// Cross-tenant: the same id under another workspace is not found.
	_, err = r.Call(context.Background(), otherWS, allScopes, "get_lead", json.RawMessage(`{"lead_id":"`+leadID+`"}`))
	wantUserErr(t, err, "not found")
}

func TestCreateListener(t *testing.T) {
	r, q, _, _ := newTestRegistry()
	out, err := call(t, r, "create_listener", `{"name":" Buyers ","looks_for":"people asking for CRMs","skips":"job posts","keywords":["crm alternative","dupe"," "],"platforms":["reddit"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if out["listener"].(map[string]any)["looks_for"] != "people asking for CRMs" || q.intent == nil || q.intent.WorkspaceID != ws {
		t.Fatalf("intent not saved: %v", out)
	}
	if len(q.createdKW) != 1 || q.createdKW[0].Term != "crm alternative" || q.createdKW[0].ProfileID != profID || q.createdKW[0].Platforms[0] != "reddit" {
		t.Fatalf("keywords: %+v", q.createdKW)
	}
	if len(out["skipped_keywords"].([]any)) != 1 {
		t.Fatalf("duplicate should be reported as skipped: %v", out)
	}
	for _, tt := range []struct{ args, want string }{
		{`{}`, "name"},
		{`{"name":"x","platforms":["myspace"]}`, "platform"},
		{`{"name":"x","keywords":["` + strings.Repeat("k,", 60) + `"]}`, "keyword"},
		{`{"name":"` + strings.Repeat("n", 201) + `"}`, "name"},
	} {
		_, err := call(t, r, "create_listener", tt.args)
		wantUserErr(t, err, tt.want)
	}
	tooMany := make([]string, maxKeywords+1)
	for i := range tooMany {
		tooMany[i] = `"k` + strings.Repeat("x", i) + `"`
	}
	_, err = call(t, r, "create_listener", `{"name":"x","keywords":[`+strings.Join(tooMany, ",")+`]}`)
	wantUserErr(t, err, "keywords")
}

func TestTuneListener(t *testing.T) {
	r, q, _, _ := newTestRegistry()
	out, err := call(t, r, "tune_listener", `{"listener_id":"`+profID+`","skips":"recruiters","add_keywords":["new term"],"remove_keywords":["crm alternative"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if q.intent.LooksFor != "old" || q.intent.Skips != "recruiters" {
		t.Fatalf("omitted fields must be kept: %+v", q.intent)
	}
	// Only the keyword on this listener is removed (case-insensitive), not the same term on another one.
	if len(q.deletedKW) != 1 || q.deletedKW[0] != kwID {
		t.Fatalf("deleted %v", q.deletedKW)
	}
	if len(q.createdKW) != 1 || q.createdKW[0].Term != "new term" {
		t.Fatalf("created %+v", q.createdKW)
	}
	if out["removed_keywords"].(float64) != 1 {
		t.Fatalf("out %v", out)
	}
	for _, tt := range []struct{ args, want string }{
		{`{"listener_id":"` + profID + `"}`, "nothing to change"},
		{`{"listener_id":"` + missing + `","skips":"x"}`, "not found"},
		{`{"listener_id":"bad","skips":"x"}`, "listener_id"},
		{`{"listener_id":"` + profID + `","looks_for":"` + strings.Repeat("x", 2001) + `"}`, "looks_for"},
	} {
		_, err := call(t, r, "tune_listener", tt.args)
		wantUserErr(t, err, tt.want)
	}
}

func TestMute(t *testing.T) {
	r, _, _, mutes := newTestRegistry()
	out, err := call(t, r, "mute", `{"kind":"author","value":" spammer ","platform":"reddit"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out["muted"] != true || len(mutes.got) != 1 || mutes.got[0].Value != "spammer" {
		t.Fatalf("out %v mutes %+v", out, mutes.got)
	}
	for _, tt := range []struct{ args, want string }{
		{`{"kind":"user","value":"x"}`, "kind"},
		{`{"kind":"author","value":""}`, "value"},
		{`{"kind":"author","value":"x","platform":"myspace"}`, "platform"},
	} {
		_, err := call(t, r, "mute", tt.args)
		wantUserErr(t, err, tt.want)
	}
	if _, err := call(t, r, "mute", `{"kind":"domain","value":"boom"}`); err == nil || errors.As(err, new(*Error)) {
		t.Fatalf("store failure must be an internal error, got %v", err)
	}
	// Default store (mute lists not built yet) is a safe no-op.
	noop := New(Deps{Q: &fakeQ{}})
	out2, err := noop.Call(context.Background(), ws, allScopes, "mute", json.RawMessage(`{"kind":"topic","value":"jobs"}`))
	if err != nil || out2.(map[string]any)["muted"] != false {
		t.Fatalf("noop mute: %v %v", out2, err)
	}
}

func TestDraftReply(t *testing.T) {
	r, _, _, _ := newTestRegistry()
	out, err := call(t, r, "draft_reply", `{"mention_id":"`+mentID+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out["should_reply"] != true || out["reply"].(map[string]any)["content"] != "drafted" {
		t.Fatalf("out %v", out)
	}
	_, err = call(t, r, "draft_reply", `{"mention_id":"`+missing+`"}`)
	wantUserErr(t, err, "not found")
	_, err = New(Deps{Q: &fakeQ{}}).Call(context.Background(), ws, allScopes, "draft_reply", json.RawMessage(`{"mention_id":"`+mentID+`"}`))
	wantUserErr(t, err, "not available")
}

func TestQueueReply(t *testing.T) {
	r, q, _, _ := newTestRegistry()
	out, err := call(t, r, "queue_reply", `{"reply_id":"`+replyID+`","content":"edited text"}`)
	if err != nil {
		t.Fatal(err)
	}
	if q.approved == nil || q.approved.EditedContent.String != "edited text" || q.approved.WorkspaceID != ws {
		t.Fatalf("approve %+v", q.approved)
	}
	if out["reply"].(map[string]any)["status"] != "approved" {
		t.Fatalf("out %v", out)
	}
	_, err = call(t, r, "queue_reply", `{"reply_id":"`+missing+`"}`)
	wantUserErr(t, err, "not found")
	_, err = call(t, r, "queue_reply", `{"reply_id":"`+replyID+`","content":"   "}`)
	wantUserErr(t, err, "content")
	q.replyStatus = "posted"
	_, err = call(t, r, "queue_reply", `{"reply_id":"`+replyID+`"}`)
	wantUserErr(t, err, "already posted")
}

func TestSetStageAndPushToCRM(t *testing.T) {
	r, q, crm, _ := newTestRegistry()
	out, err := call(t, r, "set_stage", `{"lead_id":"`+leadID+`","stage":"qualified"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out["lead"].(map[string]any)["stage"] != "qualified" || q.leadEvents != 1 {
		t.Fatalf("out %v events %d", out, q.leadEvents)
	}
	if len(crm.events) != 1 || crm.events[0].Type != crmsync.EventLeadStageChanged || crm.events[0].PrevStage != "prospect" {
		t.Fatalf("crm %+v", crm.events)
	}
	_, err = call(t, r, "set_stage", `{"lead_id":"`+leadID+`","stage":"won"}`)
	wantUserErr(t, err, "stage")
	_, err = call(t, r, "set_stage", `{"lead_id":"`+missing+`","stage":"lost"}`)
	wantUserErr(t, err, "not found")

	if _, err := call(t, r, "push_to_crm", `{"lead_id":"`+leadID+`"}`); err != nil {
		t.Fatal(err)
	}
	if len(crm.events) != 2 || crm.events[1].Type != crmsync.EventLeadCreated || crm.events[1].LeadID != leadID {
		t.Fatalf("crm %+v", crm.events)
	}
	_, err = call(t, r, "push_to_crm", `{"lead_id":"`+missing+`"}`)
	wantUserErr(t, err, "not found")
	_, err = New(Deps{Q: &fakeQ{}}).Call(context.Background(), ws, allScopes, "push_to_crm", json.RawMessage(`{"lead_id":"`+leadID+`"}`))
	wantUserErr(t, err, "not configured")
}

func TestReports(t *testing.T) {
	r, q, _, _ := newTestRegistry()
	out, err := call(t, r, "attribution_report", `{"days":90}`)
	if err != nil {
		t.Fatal(err)
	}
	if q.lastLimit != 90 {
		t.Fatalf("days %d", q.lastLimit)
	}
	tot := out["totals"].(map[string]any)
	if tot["revenue_cents"].(float64) != 12900 || tot["clicks"].(float64) != 35 || len(out["by_platform"].([]any)) != 2 {
		t.Fatalf("out %v", out)
	}
	if _, err := call(t, r, "attribution_report", `{}`); err != nil || q.lastLimit != 30 {
		t.Fatalf("default days: %d %v", q.lastLimit, err)
	}
	_, err = call(t, r, "attribution_report", `{"days":0}`)
	wantUserErr(t, err, "days")

	out, err = call(t, r, "weekly_summary", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if q.lastCall != "ListMentionsLeadsReady" {
		t.Fatalf("top leads not loaded: %s", q.lastCall)
	}
	h := out["headline"].(string)
	for _, s := range []string{"12 high-intent", "4 complaints", "3 new leads", "$129.00 attributed"} {
		if !strings.Contains(h, s) {
			t.Fatalf("headline %q missing %q", h, s)
		}
	}
	_, err = call(t, r, "weekly_summary", `{"days":400}`)
	wantUserErr(t, err, "days")
}
