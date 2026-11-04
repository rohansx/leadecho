package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"

	"leadecho/internal/clientip"
	"leadecho/internal/database"
)

// Config wires the built-in auth (email/password + OAuth).
type Config struct {
	JWTSecret    string
	FrontendURL  string
	ResendAPIKey string
	// RequireVerification: password accounts must verify their email before
	// they can sign in. Only meaningful when email can actually be sent.
	RequireVerification bool
	SecureCookie        bool
	// TrustedProxyHops: see clientip.From. 0 = use the TCP peer address.
	TrustedProxyHops int
}

// EmailHandler serves email/password auth and the shared session endpoints.
type EmailHandler struct {
	queries      *database.Queries
	jwtSecret    string
	frontendURL  string
	secure       bool
	requireVerif bool
	mail         mailer
	counter      Counter // nil disables rate limiting
	versions     *SessionVersions
	proxyHops    int
	logger       zerolog.Logger
}

func NewEmailHandler(cfg Config, q *database.Queries, counter Counter, versions *SessionVersions, logger zerolog.Logger) *EmailHandler {
	front := strings.TrimRight(cfg.FrontendURL, "/")
	return &EmailHandler{
		queries:      q,
		jwtSecret:    cfg.JWTSecret,
		frontendURL:  front,
		secure:       cfg.SecureCookie,
		requireVerif: cfg.RequireVerification && cfg.ResendAPIKey != "",
		mail:         mailer{apiKey: cfg.ResendAPIKey, frontendURL: front, from: "LeadEcho <hello@leadecho.io>"},
		counter:      counter,
		versions:     versions,
		proxyHops:    cfg.TrustedProxyHops,
		logger:       logger.With().Str("component", "auth").Logger(),
	}
}

// RequiresVerification reports whether new password accounts must verify email.
func (h *EmailHandler) RequiresVerification() bool { return h.requireVerif }

const (
	minPasswordLen = 8
	maxPasswordLen = 72 // bcrypt ignores bytes beyond 72
)

func validatePassword(p string) error {
	if len(p) < minPasswordLen {
		return fmt.Errorf("password must be at least %d characters", minPasswordLen)
	}
	if len(p) > maxPasswordLen {
		return fmt.Errorf("password must be at most %d bytes", maxPasswordLen)
	}
	return nil
}

func normalizeEmail(e string) string { return strings.TrimSpace(strings.ToLower(e)) }

// dummyHash is compared against when a login has no user/password, so a
// missing account costs the same bcrypt time as a wrong password.
var dummyHash = sync.OnceValue(func() []byte {
	h, _ := bcrypt.GenerateFromPassword([]byte("leadecho-timing-equalizer"), bcrypt.DefaultCost)
	return h
})

// requireJSON rejects non-JSON bodies with 415. A cross-site HTML form can only
// send form/text content types without a CORS preflight, so this blocks
// login/logout CSRF on the cookie-authenticated auth endpoints.
func requireJSON(w http.ResponseWriter, r *http.Request) bool {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "Content-Type must be application/json"})
		return false
	}
	return true
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if !requireJSON(w, r) {
		return false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return false
	}
	return true
}

// Register creates a password account (and its workspace).
func (h *EmailHandler) Register(w http.ResponseWriter, r *http.Request) {
	if h.limited(w, r, "register:ip:"+h.ip(r), ipRule) {
		return
	}
	var body struct {
		Email, Password, Name string
	}
	if !decode(w, r, &body) {
		return
	}
	body.Email = normalizeEmail(body.Email)
	body.Name = strings.TrimSpace(body.Name)
	if body.Email == "" || body.Name == "" || !strings.Contains(body.Email, "@") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and a valid email are required"})
		return
	}
	if err := validatePassword(body.Password); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if _, err := h.queries.FindUserByEmail(r.Context(), body.Email); err == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "email already registered"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		h.fail(w, err, "hash password")
		return
	}
	user, ws, err := h.createAccount(r.Context(), "email_"+body.Email, body.Email, body.Name, "", string(hash), false)
	if err != nil {
		h.fail(w, err, "create account")
		return
	}

	if h.requireVerif {
		if err := h.sendToken(r.Context(), user, tokenKindVerify); err != nil {
			h.logger.Error().Err(err).Str("user_id", user.ID).Msg("send verification email")
		}
		writeJSON(w, http.StatusCreated, map[string]any{"verification_required": true, "email": user.Email})
		return
	}
	if !h.startSession(w, user, ws.ID, 0) {
		return
	}
	go h.mail.welcome(context.Background(), user.Name, user.Email)
	writeJSON(w, http.StatusCreated, userJSON(user, ws.ID))
}

// Login authenticates with email/password.
func (h *EmailHandler) Login(w http.ResponseWriter, r *http.Request) {
	if h.limited(w, r, "login:ip:"+h.ip(r), ipRule) {
		return
	}
	var body struct{ Email, Password string }
	if !decode(w, r, &body) {
		return
	}
	body.Email = normalizeEmail(body.Email)
	if body.Email == "" || body.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email and password are required"})
		return
	}
	// Count the attempt before the bcrypt check so parallel guesses can't race
	// past the limit; a successful login clears the counter.
	emailKey := "login:email:" + body.Email
	if h.limited(w, r, emailKey, emailRule) {
		return
	}

	user, err := h.queries.FindUserByEmail(r.Context(), body.Email)
	hash := dummyHash()
	if err == nil && user.PasswordHash.Valid {
		hash = []byte(user.PasswordHash.String)
	}
	match := bcrypt.CompareHashAndPassword(hash, []byte(body.Password)) == nil
	if err != nil || !user.PasswordHash.Valid || !match {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid email or password"})
		return
	}

	st, err := h.queries.GetUserAuthState(r.Context(), user.ID)
	if err != nil {
		h.fail(w, err, "load auth state")
		return
	}
	if h.requireVerif && !st.EmailVerified {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "email not verified", "code": "email_unverified"})
		return
	}
	if !h.startSession(w, user, user.WorkspaceID, st.SessionVersion) {
		return
	}
	h.resetLimit(r.Context(), emailKey)
	writeJSON(w, http.StatusOK, userJSON(user, user.WorkspaceID))
}

// Verify consumes an email-verification link. It deliberately does NOT sign
// the clicker in: if an attacker registered the victim's email, the victim
// clicking the link must not land in the attacker's account. It bumps the
// session version (revoking any session made with the unproven account) and
// sends the user to sign in.
func (h *EmailHandler) Verify(w http.ResponseWriter, r *http.Request) {
	userID, err := h.queries.ConsumeAuthToken(r.Context(), tokenKindVerify, hashToken(r.URL.Query().Get("token")))
	if err != nil {
		http.Redirect(w, r, h.frontendURL+"/login?error=invalid_token", http.StatusFound)
		return
	}
	if err := h.queries.VerifyEmailAndRevoke(r.Context(), userID); err != nil {
		h.logger.Error().Err(err).Str("user_id", userID).Msg("verify email")
		http.Redirect(w, r, h.frontendURL+"/login?error=auth_failed", http.StatusFound)
		return
	}
	h.versions.Forget(userID)
	if user, err := h.queries.GetUser(r.Context(), userID); err == nil {
		go h.mail.welcome(context.Background(), user.Name, user.Email)
	}
	http.Redirect(w, r, h.frontendURL+"/login?verified=1", http.StatusFound)
}

// ResendVerification re-sends the verify link. Always 200 (no enumeration).
func (h *EmailHandler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	h.emailAction(w, r, "resend", func(ctx context.Context, u database.User) error {
		st, err := h.queries.GetUserAuthState(ctx, u.ID)
		if err != nil || st.EmailVerified || !h.requireVerif {
			return err
		}
		return h.sendToken(ctx, u, tokenKindVerify)
	})
}

// ForgotPassword emails a reset link. Always 200 (no enumeration).
func (h *EmailHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	h.emailAction(w, r, "forgot", func(ctx context.Context, u database.User) error {
		return h.sendToken(ctx, u, tokenKindReset)
	})
}

// emailAction runs fn for an existing user, rate-limited, always answering 200.
func (h *EmailHandler) emailAction(w http.ResponseWriter, r *http.Request, name string, fn func(context.Context, database.User) error) {
	if h.limited(w, r, name+":ip:"+h.ip(r), ipRule) {
		return
	}
	var body struct{ Email string }
	if !decode(w, r, &body) {
		return
	}
	email := normalizeEmail(body.Email)
	ok := map[string]string{"status": "if that account exists, we've sent an email"}
	if email == "" {
		writeJSON(w, http.StatusOK, ok)
		return
	}
	if h.limited(w, r, name+":email:"+email, emailRule) {
		return
	}
	// Lookup + token + Resend call run in the background so the response time
	// doesn't reveal whether the account exists.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		u, err := h.queries.FindUserByEmail(ctx, email)
		if err != nil {
			if !isNotFound(err) {
				h.logger.Error().Err(err).Str("action", name).Msg("auth email action: lookup")
			}
			return
		}
		if err := fn(ctx, u); err != nil {
			h.logger.Error().Err(err).Str("user_id", u.ID).Str("action", name).Msg("auth email action")
		}
	}()
	writeJSON(w, http.StatusOK, ok)
}

// ResetPassword sets a new password from a reset token and revokes all sessions.
func (h *EmailHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	if h.limited(w, r, "reset:ip:"+h.ip(r), ipRule) {
		return
	}
	var body struct{ Token, Password string }
	if !decode(w, r, &body) {
		return
	}
	if err := validatePassword(body.Password); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	userID, err := h.queries.ConsumeAuthToken(r.Context(), tokenKindReset, hashToken(body.Token))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reset link is invalid or expired", "code": "invalid_token"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		h.fail(w, err, "hash password")
		return
	}
	if _, err := h.queries.ResetPassword(r.Context(), userID, string(hash)); err != nil {
		h.fail(w, err, "reset password")
		return
	}
	h.versions.Forget(userID)
	_ = h.queries.InvalidateAuthTokens(r.Context(), userID, tokenKindReset)
	ClearSessionCookie(w, h.secure)
	writeJSON(w, http.StatusOK, map[string]string{"status": "password updated"})
}

// Me returns the current authenticated user.
func (h *EmailHandler) Me(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	claims, err := ValidateToken(h.jwtSecret, cookie.Value)
	if err != nil || !h.versions.Valid(r.Context(), claims) {
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
	if !requireJSON(w, r) {
		return
	}
	ClearSessionCookie(w, h.secure)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

// ---- shared helpers ------------------------------------------------------

func userJSON(u database.User, workspaceID string) map[string]any {
	return map[string]any{
		"user_id": u.ID, "workspace_id": workspaceID,
		"email": u.Email, "name": u.Name, "role": string(u.Role),
	}
}

func (h *EmailHandler) startSession(w http.ResponseWriter, u database.User, workspaceID string, version int32) bool {
	token, err := IssueToken(h.jwtSecret, u.ID, workspaceID, u.Email, u.Name, string(u.Role), version)
	if err != nil {
		h.fail(w, err, "issue session")
		return false
	}
	SetSessionCookie(w, token, h.secure)
	return true
}

func (h *EmailHandler) fail(w http.ResponseWriter, err error, what string) {
	h.logger.Error().Err(err).Msg(what)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "something went wrong, please try again"})
}

// createAccount makes a workspace + admin user. verified marks the email verified.
func (h *EmailHandler) createAccount(ctx context.Context, externalID, email, name, avatar, passwordHash string, verified bool) (database.User, database.Workspace, error) {
	ws, err := h.queries.CreateWorkspace(ctx, database.CreateWorkspaceParams{
		ClerkOrgID: externalID + "_org",
		Name:       name + "'s Workspace",
		Slug:       "ws-" + randomHex(6),
	})
	if err != nil {
		return database.User{}, database.Workspace{}, fmt.Errorf("create workspace: %w", err)
	}
	user, err := h.queries.CreateUser(ctx, database.CreateUserParams{
		ClerkUserID:  externalID,
		WorkspaceID:  ws.ID,
		Email:        email,
		Name:         name,
		AvatarUrl:    pgtype.Text{String: avatar, Valid: avatar != ""},
		Role:         database.UserRoleAdmin,
		PasswordHash: pgtype.Text{String: passwordHash, Valid: passwordHash != ""},
	})
	if err != nil {
		return database.User{}, database.Workspace{}, fmt.Errorf("create user: %w", err)
	}
	if verified {
		if err := h.queries.SetEmailVerified(ctx, user.ID); err != nil {
			return database.User{}, database.Workspace{}, fmt.Errorf("mark verified: %w", err)
		}
	}
	h.logger.Info().Str("user_id", user.ID).Str("source", strings.SplitN(externalID, "_", 2)[0]).Msg("new user registered")
	return user, ws, nil
}

// sendToken issues a fresh single-use token (invalidating older ones of the same kind) and emails it.
func (h *EmailHandler) sendToken(ctx context.Context, u database.User, kind string) error {
	if !h.mail.enabled() {
		return nil
	}
	raw, hash, err := newToken()
	if err != nil {
		return err
	}
	ttl := verifyTokenTTL
	if kind == tokenKindReset {
		ttl = resetTokenTTL
	}
	if err := h.queries.InvalidateAuthTokens(ctx, u.ID, kind); err != nil {
		return err
	}
	if err := h.queries.CreateAuthToken(ctx, u.ID, kind, hash, time.Now().Add(ttl)); err != nil {
		return err
	}
	if kind == tokenKindReset {
		return h.mail.reset(ctx, u.Email, raw)
	}
	return h.mail.verify(ctx, u.Email, raw)
}

// limited counts this request against a fixed-window limit and writes 429
// when over it. Fails open on counter errors so a Redis blip doesn't lock
// everyone out.
func (h *EmailHandler) limited(w http.ResponseWriter, r *http.Request, key string, rl rule) bool {
	if h.counter == nil {
		return false
	}
	n, ttl, err := h.counter.Incr(r.Context(), rateLimitPrefix+key, rl.window)
	if err != nil {
		h.logger.Warn().Err(err).Str("key", key).Msg("rate limiter unavailable, allowing request")
		return false
	}
	if n <= rl.limit {
		return false
	}
	if ttl <= 0 {
		ttl = rl.window
	}
	w.Header().Set("Retry-After", strconv.Itoa(int(ttl.Round(time.Second).Seconds())))
	writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many attempts, please try again later"})
	return true
}

func (h *EmailHandler) resetLimit(ctx context.Context, key string) {
	if h.counter == nil {
		return
	}
	if err := h.counter.Reset(ctx, rateLimitPrefix+key); err != nil {
		h.logger.Warn().Err(err).Msg("rate limiter reset failed")
	}
}

func (h *EmailHandler) ip(r *http.Request) string { return clientip.From(r, h.proxyHops) }

var errNotFound = pgx.ErrNoRows

func isNotFound(err error) bool { return errors.Is(err, errNotFound) }
