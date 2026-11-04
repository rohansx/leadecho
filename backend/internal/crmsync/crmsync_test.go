package crmsync

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
)

// fakeDB records Exec calls (MergeLeadMetadata); other methods are unused here.
type fakeDB struct {
	mu    sync.Mutex
	execs [][]any
}

func (f *fakeDB) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.execs = append(f.execs, args)
	return pgconn.CommandTag{}, nil
}
func (f *fakeDB) Query(context.Context, string, ...any) (pgx.Rows, error) { panic("unused") }
func (f *fakeDB) QueryRow(context.Context, string, ...any) pgx.Row        { panic("unused") }

type call struct{ method, path, auth, body string }

func vendor(t *testing.T, respond map[string]string) (*httptest.Server, *[]call) {
	t.Helper()
	var calls []call
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		calls = append(calls, call{r.Method, r.URL.Path, r.Header.Get("Authorization"), string(b)})
		w.Write([]byte(respond[r.Method+" "+r.URL.Path]))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func testDispatcher(db *fakeDB, base string) *Dispatcher {
	d := NewDispatcher(database.New(db), nil, zerolog.Nop())
	d.api = http.DefaultClient
	d.hubspot, d.attio = base, base
	return d
}

func lead(meta string) database.Lead {
	return database.Lead{
		ID: "11111111-2222-3333-4444-555555555555", Stage: database.LeadStageProspect,
		ContactName: pgtype.Text{String: "Ada Lovelace", Valid: true},
		Username:    pgtype.Text{String: "ada", Valid: true},
		Metadata:    []byte(meta),
	}
}

func TestSign(t *testing.T) {
	// echo -n '1700000000.{"a":1}' | openssl dgst -sha256 -hmac secret
	const want = "49f24e537407743fa4a0242bb63b94b9a47ee99cbbe071ccd8a22550ae411686"
	if got := Sign("secret", "1700000000", []byte(`{"a":1}`)); got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestValidateWebhookURL(t *testing.T) {
	for raw, ok := range map[string]bool{
		"https://example.com/hook": true,
		"http://example.com/hook":  false,
		"javascript:alert(1)":      false,
		"not a url":                false,
	} {
		if err := ValidateWebhookURL(raw); (err == nil) != ok {
			t.Errorf("%q: err=%v want ok=%v", raw, err, ok)
		}
	}
}

func TestHubSpotCreateThenPatch(t *testing.T) {
	srv, calls := vendor(t, map[string]string{"POST /crm/v3/objects/contacts": `{"id":"901"}`})
	db := &fakeDB{}
	d := testDispatcher(db, srv.URL)
	ctx := context.Background()

	l := lead(`{}`)
	if err := d.syncHubSpot(ctx, "ws", "tok", Event{Type: EventLeadCreated}, l, toPayload(l)); err != nil {
		t.Fatal(err)
	}
	c := (*calls)[0]
	if c.auth != "Bearer tok" || !strings.Contains(c.body, `"firstname":"Ada"`) || !strings.Contains(c.body, `"lifecyclestage":"lead"`) {
		t.Fatalf("unexpected create call: %+v", c)
	}
	if len(db.execs) != 1 || !strings.Contains(string(db.execs[0][2].([]byte)), `"hubspot_contact_id":"901"`) {
		t.Fatalf("contact id not persisted: %+v", db.execs)
	}

	l = lead(`{"hubspot_contact_id":"901"}`)
	l.Stage = database.LeadStageConverted
	if err := d.syncHubSpot(ctx, "ws", "tok", Event{Type: EventConversion}, l, toPayload(l)); err != nil {
		t.Fatal(err)
	}
	c = (*calls)[1]
	if c.method != http.MethodPatch || c.path != "/crm/v3/objects/contacts/901" || !strings.Contains(c.body, `"customer"`) {
		t.Fatalf("unexpected patch call: %+v", c)
	}
}

func TestHubSpotAdoptsExistingContactOn409(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"status":"error","message":"Contact already exists. Existing ID: 4242","category":"CONFLICT"}`))
	}))
	defer srv.Close()
	db := &fakeDB{}
	d := testDispatcher(db, srv.URL)

	l := lead(`{}`)
	if err := d.syncHubSpot(context.Background(), "ws", "tok", Event{Type: EventLeadCreated}, l, toPayload(l)); err != nil {
		t.Fatal(err)
	}
	if len(db.execs) != 1 || !strings.Contains(string(db.execs[0][2].([]byte)), `"4242"`) {
		t.Fatalf("existing id not adopted: %+v", db.execs)
	}
}

func TestAttioCreatesPersonAndNote(t *testing.T) {
	srv, calls := vendor(t, map[string]string{"POST /v2/objects/people/records": `{"data":{"id":{"record_id":"rec-1"}}}`})
	db := &fakeDB{}
	d := testDispatcher(db, srv.URL)

	l := lead(`{}`)
	ev := Event{Type: EventConversion, ConvType: "purchase", RevenueCents: 34800}
	if err := d.syncAttio(context.Background(), "ws", "tok", ev, l, toPayload(l)); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 2 || (*calls)[1].path != "/v2/notes" {
		t.Fatalf("want person + note, got %+v", *calls)
	}
	if !strings.Contains((*calls)[0].body, `"name":{"first_name":"Ada","full_name":"Ada Lovelace","last_name":"Lovelace"}`) {
		t.Fatalf("attio name must use object syntax: %s", (*calls)[0].body)
	}
	var note struct {
		Data map[string]string `json:"data"`
	}
	_ = json.Unmarshal([]byte((*calls)[1].body), &note)
	if note.Data["parent_record_id"] != "rec-1" || !strings.Contains(note.Data["content"], "$348.00") {
		t.Fatalf("bad note: %+v", note.Data)
	}
}
