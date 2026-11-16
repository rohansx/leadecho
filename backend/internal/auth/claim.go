package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"leadecho/internal/database"
	"leadecho/internal/lobby"
)

// Claimable accounts. An operator builds an account for a prospect under a
// placeholder email; Lobby verifies the prospect's email and sends them to
// GET /api/v1/auth/claim?claim=<code>. We redeem the code server side, swap in
// the verified email and sign them in. With Keycloak (OIDC) on, the temp
// account's Keycloak user is deleted so the operator's password dies with it;
// the prospect later signs in through Keycloak (Google, or sign up with the
// same email) and is linked by email. Without Keycloak they pick a password.

type claimRedeemer interface {
	Redeem(ctx context.Context, code string) (lobby.Redemption, error)
}

type claimStore interface {
	ClaimAccount(ctx context.Context, userID, email string) (database.User, string, error)
	SetInitialPassword(ctx context.Context, userID, hash string) (bool, error)
}

// IdPUsers removes a user from the identity provider (Keycloak).
type IdPUsers interface {
	DeleteUserByEmail(ctx context.Context, email string) error
}

type ClaimHandler struct {
	core  *EmailHandler
	lobby claimRedeemer
	store claimStore
	idp   IdPUsers // nil: no Keycloak; claimants set a LeadEcho password instead
}

func NewClaimHandler(core *EmailHandler, l *lobby.Client, idp IdPUsers) *ClaimHandler {
	return &ClaimHandler{core: core, lobby: l, store: core.queries, idp: idp}
}

const (
	claimPwCookie = "claim_pw"
	claimPwTTL    = 30 * time.Minute
)

// Redeem is the Lobby Invite URL. Every exit is a redirect, so the ?claim=
// code never stays in the address bar or history.
func (h *ClaimHandler) Redeem(w http.ResponseWriter, r *http.Request) {
	front := h.core.frontendURL
	code := r.URL.Query().Get("claim")
	if code == "" || len(code) > 512 {
		http.Redirect(w, r, front+"/login", http.StatusFound)
		return
	}
	if h.core.limited(w, r, "claim:ip:"+h.core.ip(r), ipRule) {
		return
	}
	expired := func(reason string, err error) {
		h.core.logger.Warn().Err(err).Str("reason", reason).Msg("claim refused")
		http.Redirect(w, r, front+"/login?error=claim_expired", http.StatusFound)
	}

	red, err := h.lobby.Redeem(r.Context(), code)
	if err != nil {
		expired("redeem", err)
		return
	}
	email := normalizeEmail(red.Email)
	if _, err := uuid.Parse(red.AccountRef); err != nil || !strings.Contains(email, "@") {
		expired("bad account_ref or email", err)
		return
	}
	user, placeholder, err := h.store.ClaimAccount(r.Context(), red.AccountRef, email)
	switch {
	case errors.Is(err, database.ErrEmailTaken):
		h.core.logger.Info().Str("account_ref", red.AccountRef).Msg("claim refused: email has an account")
		http.Redirect(w, r, front+"/login?error=claim_email_taken", http.StatusFound)
		return
	case errors.Is(err, pgx.ErrNoRows):
		expired("no claimable account", err)
		return
	case err != nil:
		h.core.logger.Error().Err(err).Str("account_ref", red.AccountRef).Msg("claim account")
		http.Redirect(w, r, front+"/login?error=auth_failed", http.StatusFound)
		return
	}
	h.core.versions.Forget(user.ID)
	if !h.core.startSession(w, user, user.WorkspaceID, user.SessionVersion) {
		return
	}
	if h.idp != nil {
		// The placeholder's Keycloak login no longer maps to this account (its
		// email is gone), but delete it so the temp password is dead everywhere.
		if err := h.idp.DeleteUserByEmail(r.Context(), placeholder); err != nil {
			h.core.logger.Error().Err(err).Str("user_id", user.ID).Msg("claim: delete placeholder keycloak user; remove it by hand")
		}
		h.core.logger.Info().Str("user_id", user.ID).Int64("claim_id", red.ClaimID).Msg("account claimed")
		http.Redirect(w, r, front+"/inbox", http.StatusFound)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: claimPwCookie, Value: h.passwordTicket(user.ID, user.SessionVersion, time.Now().Add(claimPwTTL)),
		Path: "/api/v1/auth/claim", MaxAge: int(claimPwTTL.Seconds()),
		HttpOnly: true, Secure: h.core.secure, SameSite: http.SameSiteLaxMode,
	})
	h.core.logger.Info().Str("user_id", user.ID).Int64("claim_id", red.ClaimID).Msg("account claimed")
	http.Redirect(w, r, front+"/set-password", http.StatusFound)
}

// SetPassword sets the first password of a just-claimed account. It needs the
// session and the short-lived ticket cookie Redeem set, so a stolen session
// alone can't plant a password.
func (h *ClaimHandler) SetPassword(w http.ResponseWriter, r *http.Request) {
	var body struct{ Password string }
	if !decode(w, r, &body) {
		return
	}
	claims := h.sessionClaims(r)
	ticket, cerr := r.Cookie(claimPwCookie)
	if claims == nil || cerr != nil || !h.validTicket(ticket.Value, claims.UserID, claims.SessionVersion, time.Now()) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "this step has expired; use \"Forgot password\" to set one"})
		return
	}
	if err := validatePassword(body.Password); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		h.core.fail(w, err, "hash password")
		return
	}
	set, err := h.store.SetInitialPassword(r.Context(), claims.UserID, string(hash))
	if err != nil {
		h.core.fail(w, err, "set initial password")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: claimPwCookie, Value: "", Path: "/api/v1/auth/claim", MaxAge: -1, HttpOnly: true, Secure: h.core.secure})
	if !set {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "this account already has a password"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "password set"})
}

func (h *ClaimHandler) sessionClaims(r *http.Request) *Claims {
	c, err := r.Cookie("session")
	if err != nil {
		return nil
	}
	claims, err := ValidateToken(h.core.jwtSecret, c.Value)
	if err != nil || !h.core.versions.Valid(r.Context(), claims) {
		return nil
	}
	return claims
}

// passwordTicket = "<unix expiry>.<hmac(user, session version, expiry)>".
func (h *ClaimHandler) passwordTicket(userID string, version int32, exp time.Time) string {
	e := strconv.FormatInt(exp.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(h.core.jwtSecret))
	fmt.Fprintf(mac, "claim_pw|%s|%d|%s", userID, version, e)
	return e + "." + hex.EncodeToString(mac.Sum(nil))
}

func (h *ClaimHandler) validTicket(ticket, userID string, version int32, now time.Time) bool {
	e, _, ok := strings.Cut(ticket, ".")
	exp, err := strconv.ParseInt(e, 10, 64)
	if !ok || err != nil || now.Unix() > exp {
		return false
	}
	return hmac.Equal([]byte(ticket), []byte(h.passwordTicket(userID, version, time.Unix(exp, 0))))
}
