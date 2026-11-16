package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// Hand-written claimable-account queries (sqlc is not used). See migration 00040.

// ErrEmailTaken: another account already uses that email.
var ErrEmailTaken = errors.New("email already belongs to another account")

func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

// CreateClaimableAccount makes a workspace and its admin user in one statement.
// verified marks the placeholder email verified (the ops "skip email
// verification" toggle) so the operator can sign in to build the account; the
// claim later replaces it with the prospect's verified email.
func (q *Queries) CreateClaimableAccount(ctx context.Context, email, name, passwordHash string, verified bool) (userID, workspaceID string, err error) {
	err = q.db.QueryRow(ctx, `
		WITH ws AS (
			INSERT INTO workspaces (clerk_org_id, name, slug)
			VALUES ('email_' || $1::text || '_org', $2::text || '''s Workspace',
			        'ws-' || substr(md5(random()::text), 1, 12))
			RETURNING id
		)
		INSERT INTO users (clerk_user_id, workspace_id, email, name, role, password_hash, email_verified, claimable)
		SELECT 'email_' || $1::text, ws.id, $1, $2, 'admin', $3, $4, true FROM ws
		RETURNING id, workspace_id`, email, name, passwordHash, verified,
	).Scan(&userID, &workspaceID)
	if isUniqueViolation(err) {
		return "", "", ErrEmailTaken
	}
	return userID, workspaceID, err
}

type ClaimableAccount struct {
	UserID        string    `json:"user_id"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	WorkspaceID   string    `json:"workspace_id"`
	WorkspaceName string    `json:"workspace_name"`
	CreatedAt     time.Time `json:"created_at"`
}

// ListClaimableAccounts: accounts waiting for a prospect, newest first.
func (q *Queries) ListClaimableAccounts(ctx context.Context) ([]ClaimableAccount, error) {
	rows, err := q.db.Query(ctx, `
		SELECT u.id, u.email, u.name, w.id, w.name, u.created_at
		FROM users u JOIN workspaces w ON w.id = u.workspace_id
		WHERE u.claimable ORDER BY u.created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ClaimableAccount{}
	for rows.Next() {
		var a ClaimableAccount
		if err := rows.Scan(&a.UserID, &a.Email, &a.Name, &a.WorkspaceID, &a.WorkspaceName, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// IsClaimable reports whether userID is an unclaimed claimable account.
func (q *Queries) IsClaimable(ctx context.Context, userID string) (bool, error) {
	var ok bool
	err := q.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1 AND claimable)`, userID).Scan(&ok)
	return ok, err
}

// ClaimAccount hands a claimable account to its prospect in one statement
// (atomic). It swaps in the verified email, clears the claimable flag and the
// operator's temporary password, and bumps session_version so every session
// made while building the account is revoked. Anything else left from setup
// that still grants access goes too: pending reset/verify tokens (sent to the
// placeholder mailbox), the workspace's API keys and its extension token.
// It also returns the placeholder email the account had before.
// pgx.ErrNoRows: no such claimable account. ErrEmailTaken: the email belongs
// to another account (the unique index decides, so a racing sign-up can't
// slip in between).
func (q *Queries) ClaimAccount(ctx context.Context, userID, email string) (User, string, error) {
	var i User
	var placeholder string
	err := q.db.QueryRow(ctx, `
		WITH old AS (
			-- snapshot read; no FOR UPDATE: locking the row here makes the UPDATE
			-- below skip it. The UPDATE's WHERE claimable decides any race.
			SELECT email FROM users WHERE id = $1 AND claimable
		), u AS (
			UPDATE users SET email = $2, clerk_user_id = 'email_' || $2::text,
			       email_verified = true, claimable = false, password_hash = NULL,
			       session_version = session_version + 1
			WHERE id = $1 AND claimable
			RETURNING id, clerk_user_id, workspace_id, email, name, avatar_url, role, is_active,
			          last_active_at, created_at, updated_at, password_hash, email_verified, session_version
		), tokens AS (
			DELETE FROM auth_tokens WHERE user_id IN (SELECT id FROM u)
		), api_keys AS (
			UPDATE workspace_api_keys SET revoked_at = NOW()
			WHERE revoked_at IS NULL AND workspace_id IN (SELECT workspace_id FROM u)
		), ext AS (
			DELETE FROM extension_tokens WHERE workspace_id IN (SELECT workspace_id FROM u)
		)
		SELECT u.*, old.email FROM u, old`,
		userID, email,
	).Scan(&i.ID, &i.ClerkUserID, &i.WorkspaceID, &i.Email, &i.Name, &i.AvatarUrl, &i.Role,
		&i.IsActive, &i.LastActiveAt, &i.CreatedAt, &i.UpdatedAt, &i.PasswordHash,
		&i.EmailVerified, &i.SessionVersion, &placeholder)
	if isUniqueViolation(err) {
		return User{}, "", ErrEmailTaken
	}
	return i, placeholder, err
}

// SetInitialPassword sets a password only on an account that has none (a just
// claimed account). Reports whether it was set.
func (q *Queries) SetInitialPassword(ctx context.Context, userID, hash string) (bool, error) {
	tag, err := q.db.Exec(ctx,
		`UPDATE users SET password_hash = $2 WHERE id = $1 AND password_hash IS NULL`, userID, hash)
	return tag.RowsAffected() == 1, err
}
