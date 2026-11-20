package db

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// User is an additional login account (see migration 0008). PasswordHash is
// never serialized.
type User struct {
	ID           int64      `json:"id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	PwdVersion   int64      `json:"-"`
	Disabled     bool       `json:"disabled"`
	CreatedBy    string     `json:"created_by"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
}

// ErrUserExists is returned when the username is taken (case-insensitively).
var ErrUserExists = errors.New("username already exists")

// ErrUserNotFound is returned when no account has the id or username.
var ErrUserNotFound = errors.New("user not found")

const userCols = `id, username, password_hash, pwd_version, disabled, created_by, created_at, updated_at, last_login_at`

func scanUser(sc interface{ Scan(...any) error }) (*User, error) {
	var u User
	var last sql.NullTime
	if err := sc.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.PwdVersion, &u.Disabled, &u.CreatedBy, &u.CreatedAt, &u.UpdatedAt, &last); err != nil {
		return nil, err
	}
	if last.Valid {
		u.LastLoginAt = &last.Time
	}
	return &u, nil
}

// ListUsers returns every account, oldest first.
func (d *DB) ListUsers() ([]*User, error) {
	rows, err := d.Query(`SELECT ` + userCols + ` FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UserByName looks an account up case-insensitively; ErrUserNotFound when absent.
func (d *DB) UserByName(username string) (*User, error) {
	u, err := scanUser(d.QueryRow(`SELECT `+userCols+` FROM users WHERE lower(username) = lower($1)`, strings.TrimSpace(username)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	return u, err
}

// UserByID returns one account; ErrUserNotFound when absent.
func (d *DB) UserByID(id int64) (*User, error) {
	u, err := scanUser(d.QueryRow(`SELECT `+userCols+` FROM users WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	return u, err
}

// CreateUser inserts an account with version 1; ErrUserExists when the name is taken.
func (d *DB) CreateUser(username, hash, createdBy string) (*User, error) {
	u, err := scanUser(d.QueryRow(`
INSERT INTO users (username, password_hash, created_by) VALUES ($1, $2, $3)
ON CONFLICT ((lower(username))) DO NOTHING
RETURNING `+userCols, strings.TrimSpace(username), hash, createdBy))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserExists
	}
	return u, err
}

// SetUserPassword stores a new hash and bumps the version in one statement, so
// the account's older sessions stop validating. It returns the updated row.
func (d *DB) SetUserPassword(id int64, hash string) (*User, error) {
	u, err := scanUser(d.QueryRow(`
UPDATE users SET password_hash = $2, pwd_version = pwd_version + 1, updated_at = now()
WHERE id = $1 RETURNING `+userCols, id, hash))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	return u, err
}

// SetUserDisabled enables or disables an account and bumps the version, so a
// disable revokes its sessions at once and a re-enable starts from a fresh login.
func (d *DB) SetUserDisabled(id int64, disabled bool) (*User, error) {
	u, err := scanUser(d.QueryRow(`
UPDATE users SET disabled = $2, pwd_version = pwd_version + 1, updated_at = now()
WHERE id = $1 RETURNING `+userCols, id, disabled))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	return u, err
}

// DeleteUser removes an account; ErrUserNotFound when absent.
func (d *DB) DeleteUser(id int64) error {
	res, err := d.Exec(`DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrUserNotFound
	}
	return err
}

// TouchUserLogin records a successful login.
func (d *DB) TouchUserLogin(id int64) error {
	_, err := d.Exec(`UPDATE users SET last_login_at = now() WHERE id = $1`, id)
	return err
}

// AuditEntry is one audit_log row.
type AuditEntry struct {
	ID       int64     `json:"id"`
	At       time.Time `json:"at"`
	Username string    `json:"username"`
	Action   string    `json:"action"`
	Path     string    `json:"path"`
	Status   int       `json:"status"`
	IP       string    `json:"ip"`
}

// AddAudit appends one audit row.
func (d *DB) AddAudit(e AuditEntry) error {
	_, err := d.Exec(`INSERT INTO audit_log (username, action, path, status, ip) VALUES ($1, $2, $3, $4, $5)`,
		e.Username, e.Action, e.Path, e.Status, e.IP)
	return err
}

// ListAudit returns the newest rows first, at most limit (1-500), optionally
// for one username.
func (d *DB) ListAudit(limit int, username string) ([]AuditEntry, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := d.Query(`
SELECT id, at, username, action, path, status, ip FROM audit_log
WHERE ($2 = '' OR lower(username) = lower($2))
ORDER BY at DESC, id DESC LIMIT $1`, limit, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.At, &e.Username, &e.Action, &e.Path, &e.Status, &e.IP); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
