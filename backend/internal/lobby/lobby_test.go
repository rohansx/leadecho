package lobby

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The body is what trylobby.app sends for POST /v1/claims: ids are numbers.
func TestCreateClaimDecodesLobbyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"id":12,"account_ref":"u1","label":"","email_lock":"","email":"","pending_email":"",
			"signup_id":null,"state":"open","claim_url":"https://trylobby.app/c/c.sig","created_at":1794835053,
			"expires_at":1797427053,"opened_at":null,"verified_at":null,"claimed_at":null}`)
	}))
	defer srv.Close()
	c, err := New(srv.URL, "sk_test").CreateClaim(context.Background(), "u1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != 12 || c.ClaimURL != "https://trylobby.app/c/c.sig" || c.State != "open" {
		t.Fatalf("got %+v", c)
	}
}
