package auth

import (
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/mpizenberg/strapontin/internal/db"
)

// InviteLifetime is how long a signup or reset link stays valid.
const InviteLifetime = 7 * 24 * time.Hour

// Invite is an unused, unexpired signup or reset link, or the first admin's
// setup link, which has no database row.
type Invite struct {
	Kind      string // signup, reset or setup
	CreatedBy int64
	ForMember int64 // reset only
	Hash      string
}

// CreateInvite stores a link and returns its token.
func CreateInvite(d *sql.DB, kind string, createdBy int64, forMember *int64) (token string, expires time.Time, err error) {
	token, hash := NewToken()
	now := time.Now().UTC()
	expires = now.Add(InviteLifetime)
	_, err = d.Exec(`INSERT INTO invite (token_hash, kind, created_by, for_member, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		hash, kind, createdBy, forMember, now.Format(time.RFC3339), expires.Format(time.RFC3339))
	return token, expires, err
}

// ErrInviteInvalid covers unknown, used and expired links alike.
var ErrInviteInvalid = errors.New("invite invalid")

// LookupInvite returns a usable invite.
func LookupInvite(q interface {
	QueryRow(string, ...any) *sql.Row
}, token string) (*Invite, error) {
	inv := Invite{Hash: HashToken(token)}
	var forMember sql.NullInt64
	err := q.QueryRow(`SELECT kind, created_by, for_member FROM invite WHERE token_hash = ? AND used_at IS NULL AND expires_at > ?`,
		inv.Hash, db.Now()).Scan(&inv.Kind, &inv.CreatedBy, &forMember)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInviteInvalid
	}
	inv.ForMember = forMember.Int64
	return &inv, err
}

// Setting reads an integer setting, with a default.
func Setting(d *sql.DB, key string, fallback int) int {
	var v string
	if d.QueryRow(`SELECT value FROM setting WHERE key = ?`, key).Scan(&v) != nil {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

// Defaults for the settings table.
const (
	DefaultMaxMembers  = 30
	DefaultInviteQuota = 1
)
