package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"

	"leadecho/internal/database"
)

// ---- fakes ----------------------------------------------------------------

type memCounter struct {
	mu  sync.Mutex
	n   map[string]int64
	err error
}

func (m *memCounter) Incr(_ context.Context, k string, w time.Duration) (int64, time.Duration, error) {
	if m.err != nil {
		return 0, 0, m.err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.n[k]++
	return m.n[k], w, nil
}

func (m *memCounter) Reset(_ context.Context, k string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.n, k)
	return m.err
}

// noRowsDB makes every query look like "not found".
type noRowsDB struct{}

type noRow struct{}

func (noRow) Scan(...any) error { return pgx.ErrNoRows }

func (noRowsDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (noRowsDB) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, pgx.ErrNoRows }
func (noRowsDB) QueryRow(context.Context, string, ...any) pgx.Row        { return noRow{} }

func jsonReq(path, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r
}

// consumeDB: ConsumeAuthToken succeeds for user "u1"; everything else is "not found".
type consumeDB struct{ noRowsDB }

type idRow string

func (r idRow) Scan(dst ...any) error { *(dst[0].(*string)) = string(r); return nil }

func (consumeDB) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	if strings.Contains(sql, "UPDATE auth_tokens") {
		return idRow("u1")
	}
	return noRow{}
}

func testHandler(c Counter) *EmailHandler {
	return NewEmailHandler(Config{JWTSecret: "s", FrontendURL: "http://app.test/"},
		database.New(noRowsDB{}), c, nil, zerolog.Nop())
}

// ---- tokens -----------------------------------------------------------------

func TestNewToken(t *testing.T) {
	raw, hash, err := newToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 43 || len(hash) != 64 || hashToken(raw) != hash {
		t.Fatalf("raw=%q hash=%q", raw, hash)
	}
	raw2, _, _ := newToken()
	if raw == raw2 {
		t.Fatal("tokens must be random")
	}
	if verifyTokenTTL != 24*time.Hour || resetTokenTTL != time.Hour {
		t.Fatal("token TTLs changed unexpectedly")
	}
}

func TestValidatePassword(t *testing.T) {
	for p, ok := range map[string]bool{
		"short":                 false,
		"longenough":            true,
		strings.Repeat("a", 72): true,
		strings.Repeat("a", 73): false,
	} {
		if (validatePassword(p) == nil) != ok {
			t.Errorf("len %d: want ok=%v", len(p), ok)
		}
	}
}

// ---- rate limiting ---------------------------------------------------------

func TestLimiterBlocksAfterLimit(t *testing.T) {
	h := testHandler(&memCounter{n: map[string]int64{}})
	r := httptest.NewRequest(http.MethodPost, "/auth/register", nil)
	for i := int64(1); i <= ipRule.limit; i++ {
		if h.limited(httptest.NewRecorder(), r, "k", ipRule) {
			t.Fatalf("blocked early at %d", i)
		}
	}
	rec := httptest.NewRecorder()
	if !h.limited(rec, r, "k", ipRule) || rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("want 429 with Retry-After, got %d %q", rec.Code, rec.Header().Get("Retry-After"))
	}
}

func TestLimiterFailsOpen(t *testing.T) {
	h := testHandler(&memCounter{err: errors.New("redis down")})
	if h.limited(httptest.NewRecorder(), httptest.NewRequest("POST", "/", nil), "k", ipRule) {
		t.Fatal("limiter must fail open when the counter errors")
	}
}

// ---- login ----------------------------------------------------------------

func TestLoginUnknownUserIsTimingSafe401(t *testing.T) {
	if _, err := bcrypt.Cost(dummyHash()); err != nil {
		t.Fatalf("dummy hash must be a real bcrypt hash: %v", err)
	}
	h := testHandler(nil)
	start := time.Now()
	rec := httptest.NewRecorder()
	h.Login(rec, jsonReq("/auth/login", `{"email":"nobody@x.io","password":"whatever123"}`))
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "invalid email or password") {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	// A bcrypt compare at DefaultCost takes tens of ms; an early return would be ~µs.
	if time.Since(start) < 10*time.Millisecond {
		t.Fatal("unknown-user login returned too fast: bcrypt compare was skipped")
	}
}

func TestLoginFailuresAreRateLimitedPerEmail(t *testing.T) {
	h := testHandler(&memCounter{n: map[string]int64{}})
	var last int
	for i := 0; i < int(emailRule.limit)+1; i++ {
		rec := httptest.NewRecorder()
		h.Login(rec, jsonReq("/auth/login", `{"email":"a@x.io","password":"wrongpass1"}`))
		last = rec.Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("want 429 after %d failures, got %d", emailRule.limit, last)
	}
}

// ---- OAuth ----------------------------------------------------------------

func TestOAuthCallbackRejectsBadState(t *testing.T) {
	h := NewOAuthHandler(NewGitHubProvider("id", "secret", "http://app.test/cb"), testHandler(nil))
	for name, cookie := range map[string]string{"missing": "", "mismatch": "other"} {
		req := httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=c&state=good", nil)
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: "oauth_state_github", Value: cookie})
		}
		rec := httptest.NewRecorder()
		h.Callback(rec, req)
		if loc := rec.Header().Get("Location"); loc != "http://app.test/login?error=invalid_state" {
			t.Errorf("%s: got %d %q", name, rec.Code, loc)
		}
	}
}

func TestOAuthLoginSetsStateCookie(t *testing.T) {
	h := NewOAuthHandler(NewGoogleProvider("cid", "secret", "http://app.test/cb"), testHandler(nil))
	rec := httptest.NewRecorder()
	h.Login(rec, httptest.NewRequest(http.MethodGet, "/auth/google", nil))
	loc := rec.Header().Get("Location")
	var state string
	for _, c := range rec.Result().Cookies() {
		if c.Name == "oauth_state_google" && c.HttpOnly {
			state = c.Value
		}
	}
	if state == "" || !strings.Contains(loc, "state="+state) || !strings.Contains(loc, "client_id=cid") {
		t.Fatalf("location %q state %q", loc, state)
	}
}

func TestGitHubUsesPrimaryVerifiedEmailOnly(t *testing.T) {
	cases := map[string]struct {
		emails string
		want   string
	}{
		"primary verified":   {`[{"email":"alt@x.io","primary":false,"verified":true},{"email":"me@x.io","primary":true,"verified":true}]`, "me@x.io"},
		"primary unverified": {`[{"email":"alt@x.io","primary":false,"verified":true},{"email":"me@x.io","primary":true,"verified":false}]`, ""},
	}
	for name, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/user":
				w.Write([]byte(`{"id":42,"login":"octo","name":"","avatar_url":"http://a"}`))
			case "/user/emails":
				w.Write([]byte(tc.emails))
			default:
				http.NotFound(w, r)
			}
		}))
		p := NewGitHubProvider("id", "s", "cb")
		p.apiURL = srv.URL
		id, err := p.fetch(context.Background(), srv.Client())
		srv.Close()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if id.Email != tc.want || id.EmailVerified != (tc.want != "") || id.ID != "42" || id.Name != "octo" {
			t.Errorf("%s: got %+v", name, id)
		}
	}
}

func TestSessionVersionClaimRoundTrip(t *testing.T) {
	tok, err := IssueToken("secret", "u", "w", "e", "n", "admin", 3)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ValidateToken("secret", tok)
	if err != nil || c.SessionVersion != 3 {
		t.Fatalf("claims %+v err %v", c, err)
	}
	var nilVersions *SessionVersions
	if !nilVersions.Valid(context.Background(), c) {
		t.Fatal("nil checker must allow (check disabled)")
	}
}

func TestAuthPostsRequireJSON(t *testing.T) {
	h := testHandler(nil)
	for name, fn := range map[string]http.HandlerFunc{
		"login": h.Login, "register": h.Register, "logout": h.Logout,
		"forgot": h.ForgotPassword, "resend": h.ResendVerification, "reset": h.ResetPassword,
	} {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("email=a@x.io&password=x"))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		fn(rec, r)
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Errorf("%s: form post got %d, want 415", name, rec.Code)
		}
	}
}

func TestVerifyDoesNotSignIn(t *testing.T) {
	h := NewEmailHandler(Config{JWTSecret: "s", FrontendURL: "http://app.test"},
		database.New(consumeDB{}), nil, nil, zerolog.Nop())
	rec := httptest.NewRecorder()
	h.Verify(rec, httptest.NewRequest(http.MethodGet, "/auth/verify?token=abc", nil))
	if loc := rec.Header().Get("Location"); loc != "http://app.test/login?verified=1" {
		t.Fatalf("got %d %q", rec.Code, loc)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session" && c.Value != "" {
			t.Fatal("verify link must not set a session")
		}
	}
}

func TestLoginThrottleCountsBeforeBcrypt(t *testing.T) {
	c := &memCounter{n: map[string]int64{}}
	h := testHandler(c)
	h.Login(httptest.NewRecorder(), jsonReq("/auth/login", `{"email":"a@x.io","password":"wrongpass1"}`))
	if got := c.n[rateLimitPrefix+"login:email:a@x.io"]; got != 1 {
		t.Fatalf("per-email counter = %d, want 1 after one attempt", got)
	}
}
