package store

import (
	"database/sql"
	"time"
)

// User is the single application account (basic auth). There is at most one row.
type User struct {
	Username     string
	PasswordHash string
	CreatedAt    int64
}

// GetUser returns the configured account, or ok=false when none exists.
func (d *DB) GetUser() (User, bool, error) {
	var u User
	err := d.sql.QueryRow(
		`SELECT username, password_hash, created_at FROM auth WHERE id = 1`).
		Scan(&u.Username, &u.PasswordHash, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	return u, true, nil
}

// SetUser creates or replaces the single account.
func (d *DB) SetUser(username, passwordHash string) error {
	_, err := d.sql.Exec(
		`INSERT INTO auth (id, username, password_hash, created_at) VALUES (1, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET username = excluded.username, password_hash = excluded.password_hash`,
		username, passwordHash, time.Now().Unix())
	return err
}

// DeleteUser removes the account and invalidates every session.
func (d *DB) DeleteUser() error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM auth WHERE id = 1`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM sessions`); err != nil {
		return err
	}
	return tx.Commit()
}

// CreateSession stores a session token valid until expiresAt.
func (d *DB) CreateSession(token string, expiresAt time.Time) error {
	_, err := d.sql.Exec(
		`INSERT INTO sessions (token, created_at, expires_at) VALUES (?, ?, ?)`,
		token, time.Now().Unix(), expiresAt.Unix())
	return err
}

// SessionValid reports whether the token exists and has not expired.
func (d *DB) SessionValid(token string, now time.Time) (bool, error) {
	if token == "" {
		return false, nil
	}
	var expiresAt int64
	err := d.sql.QueryRow(`SELECT expires_at FROM sessions WHERE token = ?`, token).Scan(&expiresAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return expiresAt > now.Unix(), nil
}

// DeleteSession removes a single session (logout).
func (d *DB) DeleteSession(token string) error {
	_, err := d.sql.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	return err
}

// ClearSessions removes every session (e.g. after a password change).
func (d *DB) ClearSessions() error {
	_, err := d.sql.Exec(`DELETE FROM sessions`)
	return err
}

// PruneSessions deletes expired sessions (housekeeping).
func (d *DB) PruneSessions(now time.Time) error {
	_, err := d.sql.Exec(`DELETE FROM sessions WHERE expires_at < ?`, now.Unix())
	return err
}
