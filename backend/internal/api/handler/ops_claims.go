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
	"leadecho/internal/lobby"
)

// Temporary (claimable) accounts: the operator makes an account with a
// placeholder email and password, signs in to set it up for a prospect, then
// opens a Lobby claim link to send them. Claiming swaps in the prospect's
// verified email and drops the temporary password (see auth.ClaimHandler).

type claimCreator interface {
	CreateClaim(ctx context.Context, accountRef, label, email string) (lobby.Claim, error)
}

// SetLobby enables claim links; without it /ops can still make temp accounts.
func (h *OpsHandler) SetLobby(l *lobby.Client) {
	if l != nil {
		h.lobby = l
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
	userID, wsID, err := h.q.CreateClaimableAccount(r.Context(), email, name, string(hash))
	if errors.Is(err, database.ErrEmailTaken) {
		writeError(w, http.StatusConflict, "an account with this email already exists")
		return
	}
	if err != nil {
		h.logger.Error().Err(err).Msg("ops: create temp account")
		writeError(w, http.StatusInternalServerError, "action failed")
		return
	}
	h.audit(r, wsID, "create_temp_account", map[string]any{"email": email, "user_id": userID})
	writeJSON(w, http.StatusCreated, map[string]string{"user_id": userID, "workspace_id": wsID, "email": email})
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
