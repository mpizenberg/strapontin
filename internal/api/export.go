package api

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/mpizenberg/strapontin/internal/auth"
	"github.com/mpizenberg/strapontin/internal/db"
)

// exportedOuting carries the production it refers to, so the export reads
// without the app.
type exportedOuting struct {
	outing
	Production *production `json:"production"`
}

// Export is everything that belongs to one member, with the catalogue
// records their outings refer to.
type Export struct {
	ExportedAt string           `json:"exported_at"`
	Member     auth.Member      `json:"member"`
	Outings    []exportedOuting `json:"outings"`
	Favorites  []favorite       `json:"favorites"`
	Notes      []exportedNote   `json:"notes"`
}

type exportedNote struct {
	Entity    string `json:"entity"`
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Body      string `json:"body"`
	UpdatedAt string `json:"updated_at"`
}

// BuildExport gathers a member's data. The CLI uses it too.
func BuildExport(d *sql.DB, memberID int64) (*Export, error) {
	e := &Export{ExportedAt: db.Now(), Notes: []exportedNote{}, Outings: []exportedOuting{}}
	if err := d.QueryRow(`SELECT id, name, login, role FROM member WHERE id = ?`, memberID).
		Scan(&e.Member.ID, &e.Member.Name, &e.Member.Login, &e.Member.Role); err != nil {
		return nil, err
	}
	list, err := outings(d, memberID, "1")
	if err != nil {
		return nil, err
	}
	productions := map[int64]*production{}
	for _, o := range list {
		p, ok := productions[o.ProductionID]
		if !ok {
			if p, err = loadProduction(d, o.ProductionID, 0); err != nil {
				return nil, err
			}
			productions[o.ProductionID] = p
		}
		e.Outings = append(e.Outings, exportedOuting{outing: o, Production: p})
	}
	if e.Favorites, err = favorites(d, memberID); err != nil {
		return nil, err
	}
	rows, err := d.Query(`
		SELECT n.entity, n.entity_id, coalesce(pe.name, v.name, p.title, ''), n.body, n.updated_at FROM note n
		LEFT JOIN person pe ON n.entity = 'person' AND pe.id = n.entity_id
		LEFT JOIN venue v ON n.entity = 'venue' AND v.id = n.entity_id
		LEFT JOIN production p ON n.entity = 'production' AND p.id = n.entity_id
		WHERE n.member_id = ? ORDER BY n.updated_at`, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var n exportedNote
		if err := rows.Scan(&n.Entity, &n.ID, &n.Name, &n.Body, &n.UpdatedAt); err != nil {
			return nil, err
		}
		e.Notes = append(e.Notes, n)
	}
	return e, rows.Err()
}

func (s *Server) exportJSON(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	e, err := BuildExport(s.db, m.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="strapontin.json"`)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(e)
}

// exportCSV writes one row per outing, for a spreadsheet.
func (s *Server) exportCSV(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	e, err := BuildExport(s.db, m.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="strapontin.csv"`)
	w.Write([]byte("\uFEFF")) // lets spreadsheets detect UTF-8
	cw := csv.NewWriter(w)
	cw.Comma = ';' // French spreadsheets expect semicolons
	cw.Write([]string{"statut", "date", "titre", "texte", "lieu", "ville", "note", "commentaire", "mise en scène", "auteur", "page"})
	status := map[string]string{"want": "envie", "seen": "vu"}
	for _, o := range e.Outings {
		p := o.Production
		var venue, city, work, rating string
		if o.Venue != nil {
			venue, city = o.Venue.Name, o.Venue.City
		} else if len(p.Runs) > 0 {
			venue, city = p.Runs[len(p.Runs)-1].Venue.Name, p.Runs[len(p.Runs)-1].Venue.City
		}
		if p.Work != nil {
			work = p.Work.Title
		}
		if o.Rating != nil {
			rating = strconv.Itoa(*o.Rating)
		}
		cw.Write([]string{status[o.Status], o.Date, p.Title, work, venue, city, rating, o.Comment,
			names(p.Credits, "director"), names(p.Credits, "author"), p.URL})
	}
	cw.Flush()
}

func names(credits []credit, role string) string {
	var out []string
	for _, c := range credits {
		if c.Role == role {
			out = append(out, c.Person.Name)
		}
	}
	return strings.Join(out, ", ")
}

// deleteAccount removes the member and their personal layer. Catalogue
// contributions stay, with created_by set to NULL by the schema.
func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	var in struct{ Password string }
	if !decode(w, r, &in) {
		return
	}
	if ok, err := auth.CheckLogin(s.db, m.Login, in.Password); err != nil {
		serverError(w, err)
		return
	} else if ok == nil {
		writeError(w, http.StatusForbidden, "Mot de passe incorrect.")
		return
	}
	err := s.inTx(func(tx *sql.Tx) error {
		if m.IsAdmin() {
			var admins int
			tx.QueryRow(`SELECT count(*) FROM member WHERE role = 'admin' AND disabled_at IS NULL`).Scan(&admins)
			if admins <= 1 {
				return errInvalid("Vous êtes le seul compte d'administration : nommez-en un autre avant de partir.")
			}
		}
		res, err := tx.Exec(`DELETE FROM member WHERE id = ?`, m.ID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return fmt.Errorf("deleted %d members", n)
		}
		return nil
	})
	if err != nil {
		writeSaveError(w, err)
		return
	}
	s.setSessionCookie(w, "", -1)
	w.WriteHeader(http.StatusNoContent)
}
