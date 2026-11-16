package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"leadecho/internal/database"
	"leadecho/internal/kcadmin"
	"leadecho/internal/lobby"
)

// Temporary (claimable) accounts: the operator makes an account with a
// placeholder email and password, signs in to set it up for a prospect, then
// opens a Lobby claim link to send them. Claiming swaps in the prospect's
// verified email and drops the temporary password (see auth.ClaimHandler).

type claimCreator interface {
	CreateClaim(ctx context.Context, accountRef, label, email string) (lobby.Claim, error)
}

// tempIdP mirrors temp accounts into the identity provider (Keycloak), so the
// operator signs in to them through the normal login.
type tempIdP interface {
	CreateTempUser(ctx context.Context, accountID, email, name, password string, verified bool) error
}

// SetLobby enables claim links; without it /ops can still make temp accounts.
func (h *OpsHandler) SetLobby(l *lobby.Client) {
	if l != nil {
		h.lobby = l
	}
}

// SetKeycloak makes temp accounts also exist in Keycloak.
func (h *OpsHandler) SetKeycloak(kc *kcadmin.Client) {
	if kc != nil {
		h.idp = kc
	}
}

// TempAccounts lists accounts not yet claimed.
func (h *OpsHandler) TempAccounts(w http.ResponseWriter, r *http.Request) {
	rows, err := h.q.ListClaimableAccounts(r.Context())
	if err != nil {
		h.logger.Error().Err(err).Msg("ops: list temp accounts")
		writeError(w, http.StatusInternalServerError, "failed to load temporary accounts")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows, "lobby_configured": h.lobby != nil})
}

// CreateTempAccount makes a claimable account the operator can sign in to.
func (h *OpsHandler) CreateTempAccount(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
		// SkipVerification marks the placeholder email verified (default on),
		// so signing in doesn't wait on a mailbox nobody reads.
		SkipVerification *bool `json:"skip_verification"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = strings.SplitN(email, "@", 2)[0]
	}
	if !strings.Contains(email, "@") || len(email) > 254 || len(name) > 100 {
		writeError(w, http.StatusBadRequest, "a valid email is required")
		return
	}
	if len(body.Password) < 8 || len(body.Password) > 72 {
		writeError(w, http.StatusBadRequest, "password must be 8 to 72 characters")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		h.logger.Error().Err(err).Msg("ops: hash temp password")
		writeError(w, http.StatusInternalServerError, "action failed")
		return
	}
	verified := body.SkipVerification == nil || *body.SkipVerification
	// Our DB first: it decides who owns an email, so a real user's address is
	// refused before any Keycloak password exists for it.
	userID, wsID, err := h.q.CreateClaimableAccount(r.Context(), email, name, string(hash), verified)
	if errors.Is(err, database.ErrEmailTaken) {
		writeError(w, http.StatusConflict, "an account with this email already exists")
		return
	}
	if err != nil {
		h.logger.Error().Err(err).Msg("ops: create temp account")
		writeError(w, http.StatusInternalServerError, "action failed")
		return
	}
	if status, msg := h.createTempIdPUser(r.Context(), userID, email, name, body.Password, verified); status != 0 {
		writeError(w, status, msg)
		return
	}
	h.audit(r, wsID, "create_temp_account", map[string]any{"email": email, "user_id": userID, "skip_verification": verified, "keycloak": h.idp != nil})
	writeJSON(w, http.StatusCreated, map[string]any{"user_id": userID, "workspace_id": wsID, "email": email, "keycloak": h.idp != nil})
}

// createTempIdPUser makes the temp account's Keycloak user; on failure it
// removes the just-created account and returns the HTTP error to send.
func (h *OpsHandler) createTempIdPUser(ctx context.Context, userID, email, name, password string, verified bool) (int, string) {
	if h.idp == nil {
		return 0, ""
	}
	err := h.idp.CreateTempUser(ctx, userID, email, name, password, verified)
	if err == nil {
		return 0, ""
	}
	if derr := h.q.DeleteClaimableAccount(context.WithoutCancel(ctx), userID); derr != nil {
		h.logger.Error().Err(derr).Str("user_id", userID).Msg("ops: roll back temp account after keycloak failure")
	}
	if errors.Is(err, kcadmin.ErrUserExists) {
		return http.StatusConflict, "the sign-in service already has a user with this email; use another placeholder email"
	}
	h.logger.Error().Err(err).Msg("ops: create keycloak user for temp account")
	return http.StatusBadGateway, "could not create the sign-in user in Keycloak"
}

// CreateClaim opens a Lobby claim link for a temp account. A new link revokes
// the account's previous one.
func (h *OpsHandler) CreateClaim(w http.ResponseWriter, r *http.Request) {
	if h.lobby == nil {
		writeError(w, http.StatusServiceUnavailable, "Lobby is not configured (set LOBBY_SECRET_KEY)")
		return
	}
	userID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(userID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid account id")
		return
	}
	var body struct {
		Label string `json:"label"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	lockEmail := strings.ToLower(strings.TrimSpace(body.Email))
	if lockEmail != "" && !strings.Contains(lockEmail, "@") {
		writeError(w, http.StatusBadRequest, "invalid email")
		return
	}
	ok, err := h.q.IsClaimable(r.Context(), userID)
	if err != nil {
		h.logger.Error().Err(err).Msg("ops: check claimable")
		writeError(w, http.StatusInternalServerError, "action failed")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "no unclaimed temporary account with that id")
		return
	}
	claim, err := h.lobby.CreateClaim(r.Context(), userID, strings.TrimSpace(body.Label), lockEmail)
	if err != nil {
		h.logger.Error().Err(err).Str("user_id", userID).Msg("ops: lobby create claim")
		writeError(w, http.StatusBadGateway, "Lobby did not create the claim link")
		return
	}
	h.audit(r, "", "create_claim_link", map[string]any{"user_id": userID, "label": body.Label, "locked_email": lockEmail, "claim_id": claim.ID})
	writeJSON(w, http.StatusOK, claim)
}
