package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"leadecho/internal/database"
)

type GoogleHandler struct {
	oauth    *oauth2.Config
	queries  *database.Queries
	jwtSecret string
	frontendURL string
	logger   zerolog.Logger
}

func NewGoogleHandler(clientID, clientSecret, redirectURL, jwtSecret, frontendURL string, q *database.Queries, logger zerolog.Logger) *GoogleHandler {
	return &GoogleHandler{
		oauth: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
		},
		queries:     q,
		jwtSecret:   jwtSecret,
		frontendURL: frontendURL,
		logger:      logger,
	}
}

// Login redirects the user to Google's consent screen.
func (h *GoogleHandler) Login(w http.ResponseWriter, r *http.Request) {
	state := randomState()
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   300,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	url := h.oauth.AuthCodeURL(state, oauth2.AccessTypeOffline)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// Callback handles the redirect from Google after consent.
func (h *GoogleHandler) Callback(w http.ResponseWriter, r *http.Request) {
	// Verify state
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	// Clear state cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	// Exchange code for token
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	token, err := h.oauth.Exchange(ctx, code)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to exchange oauth code")
		http.Error(w, "auth failed", http.StatusInternalServerError)
		return
	}

	// Get user info from Google
	userInfo, err := h.fetchGoogleUser(ctx, token)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to fetch google user info")
		http.Error(w, "auth failed", http.StatusInternalServerError)
		return
	}

	// Find or create user
	user, workspace, err := h.findOrCreateUser(ctx, userInfo)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to find/create user")
		http.Error(w, "auth failed", http.StatusInternalServerError)
		return
	}

	// Issue JWT
	jwtToken, err := IssueToken(h.jwtSecret, user.ID, workspace.ID, user.Email, user.Name, string(user.Role))
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to issue JWT")
		http.Error(w, "auth failed", http.StatusInternalServerError)
		return
	}

	// Set JWT as httpOnly cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    jwtToken,
		Path:     "/",
		MaxAge:   7 * 24 * 60 * 60, // 7 days
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	// Redirect to dashboard
	http.Redirect(w, r, h.frontendURL+"/inbox", http.StatusTemporaryRedirect)
}

// Me returns the current authenticated user.
func (h *GoogleHandler) Me(w http.ResponseWriter, r *http.Request) {
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
func (h *GoogleHandler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

type googleUserInfo struct {
	ID      string `json:"id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

func (h *GoogleHandler) fetchGoogleUser(ctx context.Context, token *oauth2.Token) (*googleUserInfo, error) {
	client := h.oauth.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return nil, fmt.Errorf("fetch userinfo: %w", err)
	}
	defer resp.Body.Close()

	var info googleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode userinfo: %w", err)
	}
	return &info, nil
}

func (h *GoogleHandler) findOrCreateUser(ctx context.Context, info *googleUserInfo) (database.User, database.Workspace, error) {
	googleID := "google_" + info.ID

	// Try to find existing user by Google ID
	users, err := h.queries.ListUsersByExternalID(ctx, googleID)
	if err != nil {
		return database.User{}, database.Workspace{}, fmt.Errorf("find user: %w", err)
	}

	if len(users) > 0 {
		user := users[0]
		workspace, err := h.queries.GetWorkspace(ctx, user.WorkspaceID)
		if err != nil {
			return database.User{}, database.Workspace{}, fmt.Errorf("find workspace: %w", err)
		}
		return user, workspace, nil
	}

	// Create new workspace + user
	slug := "ws-" + randomHex(6)
	workspace, err := h.queries.CreateWorkspace(ctx, database.CreateWorkspaceParams{
		ClerkOrgID: googleID + "_org",
		Name:       info.Name + "'s Workspace",
		Slug:       slug,
	})
	if err != nil {
		return database.User{}, database.Workspace{}, fmt.Errorf("create workspace: %w", err)
	}

	user, err := h.queries.CreateUser(ctx, database.CreateUserParams{
		ClerkUserID:  googleID,
		WorkspaceID:  workspace.ID,
		Email:        info.Email,
		Name:         info.Name,
		AvatarUrl:    pgtype.Text{String: info.Picture, Valid: info.Picture != ""},
		Role:         database.UserRoleAdmin,
		PasswordHash: pgtype.Text{},
	})
	if err != nil {
		return database.User{}, database.Workspace{}, fmt.Errorf("create user: %w", err)
	}

	h.logger.Info().Str("email", info.Email).Str("user_id", user.ID).Msg("new user registered via Google OAuth")
	return user, workspace, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func randomState() string {
	return randomHex(16)
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
