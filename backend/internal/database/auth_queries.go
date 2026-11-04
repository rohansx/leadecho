package database

import (
	"context"
	"time"
)

// Hand-written auth queries (sqlc is not used for these). See migration 00024.

// UserAuthState is the auth-only slice of a user row.
type UserAuthState struct {
	EmailVerified  bool
	SessionVersion int32
}

func (q *Queries) GetUserAuthState(ctx context.Context, userID string) (UserAuthState, error) {
	var s UserAuthState
	err := q.db.QueryRow(ctx,
		`SELECT email_verified, session_version FROM users WHERE id = $1`, userID,
	).Scan(&s.EmailVerified, &s.SessionVersion)
	return s, err
}

func (q *Queries) SetEmailVerified(ctx context.Context, userID string) error {
	_, err := q.db.Exec(ctx, `UPDATE users SET email_verified = true WHERE id = $1`, userID)
	return err
}

// ClearPasswordAndRevoke removes a password and revokes all sessions
// (pre-account-takeover guard when an unproven password account is claimed by
// a verified OAuth login), and marks the email verified.
func (q *Queries) ClearPasswordAndRevoke(ctx context.Context, userID string) error {
	_, err := q.db.Exec(ctx,
		`UPDATE users SET password_hash = NULL, email_verified = true, session_version = session_version + 1 WHERE id = $1`,
		userID)
	return err
}

// VerifyEmailAndRevoke marks the email verified (proven by the verify link)
// and revokes sessions created before the address was proven.
func (q *Queries) VerifyEmailAndRevoke(ctx context.Context, userID string) error {
	_, err := q.db.Exec(ctx,
		`UPDATE users SET email_verified = true, session_version = session_version + 1 WHERE id = $1`, userID)
	return err
}

// ResetPassword sets a new hash, marks the email verified (the reset link proved
// ownership) and bumps session_version so every existing session is revoked.
func (q *Queries) ResetPassword(ctx context.Context, userID, hash string) (int32, error) {
	var v int32
	err := q.db.QueryRow(ctx,
		`UPDATE users SET password_hash = $2, email_verified = true, session_version = session_version + 1
		 WHERE id = $1 RETURNING session_version`, userID, hash,
	).Scan(&v)
	return v, err
}

func (q *Queries) CreateAuthToken(ctx context.Context, userID, kind, tokenHash string, expiresAt time.Time) error {
	_, err := q.db.Exec(ctx,
		`INSERT INTO auth_tokens (user_id, kind, token_hash, expires_at) VALUES ($1, $2, $3, $4)`,
		userID, kind, tokenHash, expiresAt)
	return err
}

// InvalidateAuthTokens marks all unused tokens of a kind for a user as used.
func (q *Queries) InvalidateAuthTokens(ctx context.Context, userID, kind string) error {
	_, err := q.db.Exec(ctx,
		`UPDATE auth_tokens SET used_at = NOW() WHERE user_id = $1 AND kind = $2 AND used_at IS NULL`,
		userID, kind)
	return err
}

// ConsumeAuthToken atomically marks a valid (unused, unexpired) token as used
// and returns its user. pgx.ErrNoRows means invalid, expired or already used.
func (q *Queries) ConsumeAuthToken(ctx context.Context, kind, tokenHash string) (string, error) {
	var userID string
	err := q.db.QueryRow(ctx,
		`UPDATE auth_tokens SET used_at = NOW()
		 WHERE token_hash = $1 AND kind = $2 AND used_at IS NULL AND expires_at > NOW()
		 RETURNING user_id`, tokenHash, kind,
	).Scan(&userID)
	return userID, err
}
