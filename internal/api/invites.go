package api

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/mpizenberg/strapontin/internal/auth"
	"github.com/mpizenberg/strapontin/internal/db"
)

// invitesLeft is how many signup links a member may still create; -1 means no limit.
func (s *Server) invitesLeft(m *auth.Member) int {
	if m.IsAdmin() {
		return -1
	}
	var made int
	s.db.QueryRow(`SELECT count(*) FROM invite WHERE created_by = ? AND kind = 'signup'`, m.ID).Scan(&made)
	return max(0, auth.Setting(s.db, "invite_quota", auth.DefaultInviteQuota)-made)
}

type inviteLink struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at"`
}

func (s *Server) inviteURL(token string) string {
	return strings.TrimRight(s.cfg.BaseURL, "/") + "/invitation/" + token
}

func (s *Server) link(token string, expires time.Time) inviteLink {
	return inviteLink{URL: s.inviteURL(token), ExpiresAt: expires.Format(time.RFC3339)}
}

// SetupLink is the link that creates the first admin, or "" when the database
// already had an account at startup.
func (s *Server) SetupLink() string { return s.setupURL }

// lookupInvite returns a usable invite. The setup token lives only in memory
// and is accepted only while no account exists, which makes it single-use.
func (s *Server) lookupInvite(q interface {
	QueryRow(string, ...any) *sql.Row
}, token string) (*auth.Invite, error) {
	if s.setupHash == "" || auth.HashToken(token) != s.setupHash {
		return auth.LookupInvite(q, token)
	}
	var members int
	if err := q.QueryRow(`SELECT count(*) FROM member`).Scan(&members); err != nil {
		return nil, err
	}
	if members > 0 {
		return nil, auth.ErrInviteInvalid
	}
	return &auth.Invite{Kind: "setup"}, nil
}

func (s *Server) createInvite(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	if s.invitesLeft(m) == 0 {
		writeError(w, http.StatusForbidden, "Vous avez utilisé votre invitation.")
		return
	}
	token, expires, err := auth.CreateInvite(s.db, "signup", m.ID, nil)
	if err != nil {
		serverError(w, err)
		return
	}
	auth.LogEvent(s.db, &m.ID, "invite_created")
	writeJSON(w, http.StatusCreated, s.link(token, expires))
}

// checkInvite tells the invite page what to ask for.
func (s *Server) checkInvite(w http.ResponseWriter, r *http.Request) {
	inv, err := s.lookupInvite(s.db, r.PathValue("token"))
	if errors.Is(err, auth.ErrInviteInvalid) {
		writeError(w, http.StatusNotFound, "Ce lien n'est plus valable : il a déjà servi ou a expiré. Demandez-en un nouveau.")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	out := map[string]string{"kind": inv.Kind}
	switch inv.Kind {
	case "signup":
		var name string
		s.db.QueryRow(`SELECT name FROM member WHERE id = ?`, inv.CreatedBy).Scan(&name)
		out["invited_by"] = name
	case "reset":
		var login string
		s.db.QueryRow(`SELECT login FROM member WHERE id = ?`, inv.ForMember).Scan(&login)
		out["login"] = login
	}
	writeJSON(w, http.StatusOK, out)
}

// redeemInvite creates the account (signup, or setup for the first admin) or
// sets a new password (reset), then signs the person in.
func (s *Server) redeemInvite(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name, Login, Password string }
	if !decode(w, r, &in) {
		return
	}
	if len(in.Password) < auth.MinPasswordLength {
		writeError(w, http.StatusBadRequest, "Le mot de passe doit faire au moins 8 caractères.")
		return
	}
	var member *auth.Member
	var kind string
	err := s.inTx(func(tx *sql.Tx) error {
		inv, err := s.lookupInvite(tx, r.PathValue("token"))
		if err != nil {
			return err
		}
		kind = inv.Kind
		now := db.Now()
		if inv.Kind == "reset" {
			if _, err := tx.Exec(`UPDATE member SET password_hash = ? WHERE id = ?`, auth.HashPassword(in.Password), inv.ForMember); err != nil {
				return err
			}
			// A reset signs out every other device.
			if _, err := tx.Exec(`DELETE FROM session WHERE member_id = ?`, inv.ForMember); err != nil {
				return err
			}
			member = &auth.Member{ID: inv.ForMember}
		} else {
			name, login := strings.Join(strings.Fields(in.Name), " "), strings.ToLower(strings.TrimSpace(in.Login))
			if name == "" || login == "" || strings.ContainsAny(login, " \t") {
				return errInvalid("Indiquez votre nom et un identifiant sans espace.")
			}
			var members, taken int
			tx.QueryRow(`SELECT count(*), coalesce(sum(login = ?), 0) FROM member WHERE disabled_at IS NULL`, login).Scan(&members, &taken)
			if taken > 0 {
				return errInvalid("Cet identifiant est déjà pris.")
			}
			if members >= auth.Setting(s.db, "max_members", auth.DefaultMaxMembers) {
				go s.notify("Une inscription a été refusée : le nombre maximal de membres est atteint.")
				return errInvalid("Strapontin est complet pour le moment. Prévenez la personne qui vous a envoyé le lien.")
			}
			role, invitedBy := "member", sql.NullInt64{Int64: inv.CreatedBy, Valid: true}
			if inv.Kind == "setup" {
				role, invitedBy = "admin", sql.NullInt64{}
			}
			res, err := tx.Exec(`INSERT INTO member (name, login, password_hash, role, invited_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
				name, login, auth.HashPassword(in.Password), role, invitedBy, now)
			if err != nil {
				return err
			}
			id, _ := res.LastInsertId()
			member = &auth.Member{ID: id}
			if inv.Kind == "setup" {
				return nil // no invite row to mark as used
			}
		}
		_, err = tx.Exec(`UPDATE invite SET used_by = ?, used_at = ? WHERE token_hash = ?`, member.ID, now, inv.Hash)
		return err
	})
	if errors.Is(err, auth.ErrInviteInvalid) {
		writeError(w, http.StatusNotFound, "Ce lien n'est plus valable : il a déjà servi ou a expiré. Demandez-en un nouveau.")
		return
	}
	if err != nil {
		writeSaveError(w, err)
		return
	}
	if err := s.db.QueryRow(`SELECT name, login, role FROM member WHERE id = ?`, member.ID).Scan(&member.Name, &member.Login, &member.Role); err != nil {
		serverError(w, err)
		return
	}
	switch kind {
	case "setup":
		auth.LogEvent(s.db, &member.ID, "signup")
	case "signup":
		auth.LogEvent(s.db, &member.ID, "invite_used")
		auth.LogEvent(s.db, &member.ID, "signup")
		go s.notify("Un nouveau membre a rejoint Strapontin.")
	case "reset":
		auth.LogEvent(s.db, &member.ID, "invite_used")
	}
	if !s.startSession(w, r, member) {
		return
	}
	writeJSON(w, http.StatusOK, member)
}

// notify sends an ntfy push when the admin has set a topic. The message never
// carries a name: ntfy.sh is a third party.
func (s *Server) notify(message string) {
	var topic string
	if s.db.QueryRow(`SELECT value FROM setting WHERE key = 'ntfy_topic'`).Scan(&topic) != nil || topic == "" {
		return
	}
	req, _ := http.NewRequest("POST", strings.TrimRight(s.ntfyURL, "/")+"/"+topic, strings.NewReader(message))
	req.Header.Set("Title", "Strapontin")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("ntfy: %v", err)
		return
	}
	resp.Body.Close()
}
