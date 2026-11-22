package api

import (
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mpizenberg/strapontin/internal/auth"
	"github.com/mpizenberg/strapontin/internal/db"
)

// The admin page is built only from SQL over the existing tables. It shows
// counts, never comments, ratings or notes.

type adminMember struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Login     string `json:"login"`
	Role      string `json:"role"`
	InvitedBy string `json:"invited_by,omitempty"`
	Depth     int    `json:"depth"` // 0 for members nobody invited
	CreatedAt string `json:"created_at"`
	LastSeen  string `json:"last_seen,omitempty"`
	Disabled  bool   `json:"disabled"`
	Outings   int    `json:"outings"`
	Imports   int    `json:"imports"`
}

func (s *Server) adminMembers(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	list, err := s.members()
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) members() ([]adminMember, error) {
	rows, err := s.db.Query(`
		SELECT m.id, m.name, m.login, m.role, coalesce(i.name, ''), coalesce(m.invited_by, 0), m.created_at,
		  coalesce(m.last_seen_at, ''), m.disabled_at IS NOT NULL,
		  (SELECT count(*) FROM outing WHERE member_id = m.id),
		  (SELECT count(*) FROM event WHERE member_id = m.id AND kind = 'import')
		FROM member m LEFT JOIN member i ON i.id = m.invited_by ORDER BY m.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []adminMember{}
	inviter := map[int64]int64{}
	for rows.Next() {
		var a adminMember
		var by int64
		if err := rows.Scan(&a.ID, &a.Name, &a.Login, &a.Role, &a.InvitedBy, &by, &a.CreatedAt, &a.LastSeen, &a.Disabled, &a.Outings, &a.Imports); err != nil {
			return nil, err
		}
		inviter[a.ID] = by
		list = append(list, a)
	}
	for i := range list {
		for id, seen := list[i].ID, 0; inviter[id] != 0 && seen < len(list); id, seen = inviter[id], seen+1 {
			list[i].Depth++
		}
	}
	return list, rows.Err()
}

type count struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

type overview struct {
	Members     int     `json:"members"`
	Active7     int     `json:"active_7"`
	Active30    int     `json:"active_30"`
	MaxDepth    int     `json:"max_depth"`
	Signups     []count `json:"signups"` // per month
	Imports     []count `json:"imports"` // per day, last 14 days
	Fetches     []count `json:"fetches"` // per day, last 14 days
	LoginFailed int     `json:"login_failed"`
	DBBytes     int64   `json:"db_bytes"`
	Snapshots   int     `json:"snapshots"`
	SnapBytes   int64   `json:"snapshot_bytes"`
	Images      int     `json:"images"`
	ImageBytes  int64   `json:"image_bytes"`
	LastBackup  string  `json:"last_backup,omitempty"`
	Settings    struct {
		MaxMembers  int    `json:"max_members"`
		InviteQuota int    `json:"invite_quota"`
		NtfyTopic   string `json:"ntfy_topic"`
	} `json:"settings"`
}

func counts(d *sql.DB, query string, args ...any) ([]count, error) {
	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []count{}
	for rows.Next() {
		var c count
		if err := rows.Scan(&c.Key, &c.Count); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *Server) adminOverview(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	var o overview
	now := time.Now().UTC()
	since := func(days int) string { return now.AddDate(0, 0, -days).Format(time.RFC3339) }
	err := s.db.QueryRow(`SELECT
		(SELECT count(*) FROM member WHERE disabled_at IS NULL),
		(SELECT count(*) FROM member WHERE last_seen_at > ?),
		(SELECT count(*) FROM member WHERE last_seen_at > ?),
		(SELECT count(*) FROM event WHERE kind = 'login_failed' AND at > ?),
		(SELECT count(*) FROM snapshot), (SELECT coalesce(sum(length(body)), 0) FROM snapshot),
		(SELECT count(*) FROM image), (SELECT coalesce(sum(length(bytes)), 0) FROM image),
		(SELECT page_count * page_size FROM pragma_page_count(), pragma_page_size())`,
		since(7), since(30), since(7)).
		Scan(&o.Members, &o.Active7, &o.Active30, &o.LoginFailed, &o.Snapshots, &o.SnapBytes, &o.Images, &o.ImageBytes, &o.DBBytes)
	if err != nil {
		serverError(w, err)
		return
	}
	if o.Signups, err = counts(s.db, `SELECT substr(created_at, 1, 7), count(*) FROM member GROUP BY 1 ORDER BY 1`); err != nil {
		serverError(w, err)
		return
	}
	perDay := `SELECT substr(at, 1, 10), count(*) FROM event WHERE kind = ? AND at > ? GROUP BY 1 ORDER BY 1`
	if o.Imports, err = counts(s.db, perDay, "import", since(14)); err == nil {
		o.Fetches, err = counts(s.db, perDay, "fetch", since(14))
	}
	if err != nil {
		serverError(w, err)
		return
	}
	members, err := s.members()
	if err != nil {
		serverError(w, err)
		return
	}
	for _, mb := range members {
		o.MaxDepth = max(o.MaxDepth, mb.Depth)
	}
	o.LastBackup = lastBackup(s.cfg.BackupDir)
	o.Settings.MaxMembers = auth.Setting(s.db, "max_members", auth.DefaultMaxMembers)
	o.Settings.InviteQuota = auth.Setting(s.db, "invite_quota", auth.DefaultInviteQuota)
	s.db.QueryRow(`SELECT value FROM setting WHERE key = 'ntfy_topic'`).Scan(&o.Settings.NtfyTopic)
	writeJSON(w, http.StatusOK, o)
}

// lastBackup returns the modification time of the newest backup file.
func lastBackup(dir string) string {
	files, _ := filepath.Glob(filepath.Join(dir, "*.db"))
	sort.Strings(files)
	var newest time.Time
	for _, f := range files {
		if st, err := os.Stat(f); err == nil && st.ModTime().After(newest) {
			newest = st.ModTime()
		}
	}
	if newest.IsZero() {
		return ""
	}
	return newest.UTC().Format(time.RFC3339)
}

func (s *Server) adminSetDisabled(disabled bool) memberHandler {
	return func(w http.ResponseWriter, r *http.Request, m *auth.Member) {
		id, _ := pathID(r)
		if id == m.ID {
			writeError(w, http.StatusBadRequest, "Vous ne pouvez pas suspendre votre propre compte.")
			return
		}
		var at any
		if disabled {
			at = db.Now()
		}
		err := s.inTx(func(tx *sql.Tx) error {
			res, err := tx.Exec(`UPDATE member SET disabled_at = ? WHERE id = ?`, at, id)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n == 0 {
				return sql.ErrNoRows
			}
			_, err = tx.Exec(`DELETE FROM session WHERE member_id = ? AND ?`, id, disabled)
			return err
		})
		if err != nil {
			notFoundOr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// adminReset creates a one-time password reset link for a member.
func (s *Server) adminReset(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	id, _ := pathID(r)
	if !s.exists(w, "member", id) {
		return
	}
	token, expires, err := auth.CreateInvite(s.db, "reset", m.ID, &id)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.link(token, expires))
}

func (s *Server) adminSettings(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	var in struct {
		MaxMembers  *int    `json:"max_members"`
		InviteQuota *int    `json:"invite_quota"`
		NtfyTopic   *string `json:"ntfy_topic"`
	}
	if !decode(w, r, &in) {
		return
	}
	set := map[string]string{}
	if in.MaxMembers != nil {
		if *in.MaxMembers < 1 {
			writeError(w, http.StatusBadRequest, "Le nombre maximal de membres doit être positif.")
			return
		}
		set["max_members"] = strconv.Itoa(*in.MaxMembers)
	}
	if in.InviteQuota != nil {
		if *in.InviteQuota < 0 {
			writeError(w, http.StatusBadRequest, "Le quota d'invitations ne peut pas être négatif.")
			return
		}
		set["invite_quota"] = strconv.Itoa(*in.InviteQuota)
	}
	if in.NtfyTopic != nil {
		topic := strings.TrimSpace(*in.NtfyTopic)
		if strings.ContainsAny(topic, "/?# ") {
			writeError(w, http.StatusBadRequest, "Le sujet ntfy ne doit contenir ni espace ni « / ».")
			return
		}
		set["ntfy_topic"] = topic
	}
	err := s.inTx(func(tx *sql.Tx) error {
		for k, v := range set {
			if _, err := tx.Exec(`INSERT INTO setting (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
