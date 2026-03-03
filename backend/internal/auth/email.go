package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"

	"leadecho/internal/database"
)

type EmailHandler struct {
	queries      *database.Queries
	jwtSecret    string
	resendAPIKey string
	logger       zerolog.Logger
}

func NewEmailHandler(jwtSecret, resendAPIKey string, q *database.Queries, logger zerolog.Logger) *EmailHandler {
	return &EmailHandler{
		queries:      q,
		jwtSecret:    jwtSecret,
		resendAPIKey: resendAPIKey,
		logger:       logger,
	}
}

func (h *EmailHandler) sendWelcomeEmail(name, email string) {
	if h.resendAPIKey == "" {
		return
	}
	html := fmt.Sprintf(`<div style="font-family:system-ui,sans-serif;max-width:560px;margin:0 auto;padding:40px 24px;color:#0d1117">
  <h1 style="font-size:1.6rem;font-weight:800;margin-bottom:8px">Welcome to LeadEcho, %s!</h1>
  <p style="color:#636e7b;margin-bottom:24px">You're now set up to find buyers before they find your competitors.</p>
  <h2 style="font-size:1rem;font-weight:700;margin-bottom:12px">Get started in 3 steps:</h2>
  <ol style="color:#444d56;padding-left:20px;line-height:1.8">
    <li><strong>Add keywords</strong> — go to Keywords and add terms your buyers use</li>
    <li><strong>Create a pain-point profile</strong> — describe the problem you solve in plain English</li>
    <li><strong>Install the Chrome extension</strong> — collect signals as you browse and post replies with one click</li>
  </ol>
  <div style="margin-top:32px">
    <a href="https://app.leadecho.io/inbox" style="background:#27c17b;color:#0d1117;padding:12px 24px;border-radius:6px;font-weight:700;text-decoration:none;display:inline-block">Open your inbox →</a>
  </div>
  <p style="margin-top:32px;font-size:0.8rem;color:#999">Questions? Just reply to this email.</p>
</div>`, name)

	payload, _ := json.Marshal(map[string]any{
		"from":    "LeadEcho <hello@leadecho.io>",
		"to":      []string{email},
		"subject": "Welcome to LeadEcho — let's find your first lead",
		"html":    html,
	})

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(payload))
	if err != nil {
		h.logger.Warn().Err(err).Str("email", email).Msg("welcome email: failed to build request")
		return
	}
	req.Header.Set("Authorization", "Bearer "+h.resendAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.logger.Warn().Err(err).Str("email", email).Msg("welcome email: send failed")
		return
	}
	defer resp.Body.Close()
	h.logger.Info().Str("email", email).Int("status", resp.StatusCode).Msg("welcome email sent")
}

// Register creates a new user with email/password.
func (h *EmailHandler) Register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	body.Email = strings.TrimSpace(strings.ToLower(body.Email))
	body.Name = strings.TrimSpace(body.Name)

	if body.Email == "" || body.Password == "" || body.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email, password, and name are required"})
		return
	}
	if len(body.Password) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password must be at least 8 characters"})
		return
	}

	// Check if email already exists
	_, err := h.queries.FindUserByEmail(r.Context(), body.Email)
	if err == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "email already registered"})
		return
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to hash password")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "registration failed"})
		return
	}

	// Create workspace + user (same pattern as google.go)
	emailID := "email_" + body.Email
	slug := "ws-" + randomHex(6)

	workspace, err := h.queries.CreateWorkspace(r.Context(), database.CreateWorkspaceParams{
		ClerkOrgID: emailID + "_org",
		Name:       body.Name + "'s Workspace",
		Slug:       slug,
	})
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to create workspace")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "registration failed"})
		return
	}

	user, err := h.queries.CreateUser(r.Context(), database.CreateUserParams{
		ClerkUserID:  emailID,
		WorkspaceID:  workspace.ID,
		Email:        body.Email,
		Name:         body.Name,
		AvatarUrl:    pgtype.Text{},
		Role:         database.UserRoleAdmin,
		PasswordHash: pgtype.Text{String: string(hash), Valid: true},
	})
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to create user")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "registration failed"})
		return
	}

	// Issue JWT and set cookie
	token, err := IssueToken(h.jwtSecret, user.ID, workspace.ID, user.Email, user.Name, string(user.Role))
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to issue JWT")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "registration failed"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		MaxAge:   7 * 24 * 60 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	h.logger.Info().Str("email", body.Email).Str("user_id", user.ID).Msg("new user registered via email")

	// Send welcome email asynchronously (non-blocking)
	go h.sendWelcomeEmail(user.Name, user.Email)

	writeJSON(w, http.StatusCreated, map[string]any{
		"user_id":      user.ID,
		"workspace_id": workspace.ID,
		"email":        user.Email,
		"name":         user.Name,
		"role":         string(user.Role),
	})
}

// Login authenticates with email/password.
func (h *EmailHandler) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	body.Email = strings.TrimSpace(strings.ToLower(body.Email))

	if body.Email == "" || body.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email and password are required"})
		return
	}

	user, err := h.queries.FindUserByEmail(r.Context(), body.Email)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid email or password"})
		return
	}

	if !user.PasswordHash.Valid {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid email or password"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash.String), []byte(body.Password)); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid email or password"})
		return
	}

	token, err := IssueToken(h.jwtSecret, user.ID, user.WorkspaceID, user.Email, user.Name, string(user.Role))
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to issue JWT")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "login failed"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		MaxAge:   7 * 24 * 60 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":      user.ID,
		"workspace_id": user.WorkspaceID,
		"email":        user.Email,
		"name":         user.Name,
		"role":         string(user.Role),
	})
}

// Me returns the current authenticated user.
func (h *EmailHandler) Me(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	claims, err := ValidateToken(h.jwtSecret, cookie.Value)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid session"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":      claims.UserID,
		"workspace_id": claims.WorkspaceID,
		"email":        claims.Email,
		"name":         claims.Name,
		"role":         claims.Role,
	})
}

// Logout clears the session cookie.
func (h *EmailHandler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}
