package auth

import (
	"database/sql"
	"errors"
	"time"

	"github.com/mpizenberg/strapontin/internal/db"
)

// SessionLifetime is how long a session lasts without use.
const SessionLifetime = 365 * 24 * time.Hour

// Member is the signed-in member attached to a request.
type Member struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Login string `json:"login"`
	Role  string `json:"role"`
}

// IsAdmin reports whether the member has the admin role.
func (m *Member) IsAdmin() bool { return m.Role == "admin" }

// CreateSession stores a new session and returns its token.
func CreateSession(d *sql.DB, memberID int64, userAgent string) (string, error) {
	token, hash := NewToken()
	now := time.Now().UTC()
	_, err := d.Exec(`INSERT INTO session (token_hash, member_id, created_at, expires_at, user_agent) VALUES (?, ?, ?, ?, ?)`,
		hash, memberID, now.Format(time.RFC3339), now.Add(SessionLifetime).Format(time.RFC3339), userAgent)
	return token, err
}

// LookupSession returns the member owning a valid session. It renews the
// session at most once a day and records last_seen_at at most once an hour.
func LookupSession(d *sql.DB, token string) (*Member, error) {
	hash := HashToken(token)
	var m Member
	var expires string
	var lastSeen sql.NullString
	err := d.QueryRow(`
		SELECT m.id, m.name, m.login, m.role, s.expires_at, m.last_seen_at
		FROM session s JOIN member m ON m.id = s.member_id
		WHERE s.token_hash = ? AND m.disabled_at IS NULL`, hash).
		Scan(&m.ID, &m.Name, &m.Login, &m.Role, &expires, &lastSeen)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	exp, _ := time.Parse(time.RFC3339, expires)
	if now.After(exp) {
		d.Exec(`DELETE FROM session WHERE token_hash = ?`, hash)
		return nil, nil
	}
	if exp.Sub(now) < SessionLifetime-24*time.Hour {
		d.Exec(`UPDATE session SET expires_at = ? WHERE token_hash = ?`, now.Add(SessionLifetime).Format(time.RFC3339), hash)
	}
	if seen, err := time.Parse(time.RFC3339, lastSeen.String); err != nil || now.Sub(seen) > time.Hour {
		d.Exec(`UPDATE member SET last_seen_at = ? WHERE id = ?`, db.Now(), m.ID)
	}
	return &m, nil
}

// DeleteSession signs a session out.
func DeleteSession(d *sql.DB, token string) error {
	_, err := d.Exec(`DELETE FROM session WHERE token_hash = ?`, HashToken(token))
	return err
}

// CheckLogin returns the member whose login and password match, or nil.
func CheckLogin(d *sql.DB, login, password string) (*Member, error) {
	var m Member
	var hash sql.NullString
	err := d.QueryRow(`SELECT id, name, login, role, password_hash FROM member WHERE login = ? AND disabled_at IS NULL`, login).
		Scan(&m.ID, &m.Name, &m.Login, &m.Role, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		HashPassword(password) // keep timing similar for unknown logins
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !hash.Valid || !CheckPassword(hash.String, password) {
		return nil, nil
	}
	return &m, nil
}

// CreateMember inserts a member with a password and returns its id.
func CreateMember(d *sql.DB, name, login, password, role string, invitedBy *int64) (int64, error) {
	res, err := d.Exec(`INSERT INTO member (name, login, password_hash, role, invited_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		name, login, HashPassword(password), role, invitedBy, db.Now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// LogEvent records an event for the admin page. Failures are ignored:
// statistics must never break a request.
func LogEvent(d *sql.DB, memberID *int64, kind string) {
	d.Exec(`INSERT INTO event (member_id, kind, at) VALUES (?, ?, ?)`, memberID, kind, db.Now())
}
