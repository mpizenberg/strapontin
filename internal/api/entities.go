package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"unicode"

	"github.com/mpizenberg/strapontin/internal/auth"
	"github.com/mpizenberg/strapontin/internal/db"
)

type externalID struct {
	Scheme string `json:"scheme"`
	Value  string `json:"value"`
}

func externalIDs(q queryer, entity string, id int64) ([]externalID, error) {
	rows, err := q.Query(`SELECT scheme, value FROM external_id WHERE entity = ? AND entity_id = ? AND scheme <> 'url' ORDER BY scheme`, entity, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []externalID{}
	for rows.Next() {
		var e externalID
		if err := rows.Scan(&e.Scheme, &e.Value); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

// saveRevision stores the current state of a row before an edit.
func saveRevision(q queryer, entity string, id, me int64, before any) error {
	b, err := json.Marshal(before)
	if err != nil {
		return err
	}
	_, err = q.Exec(`INSERT INTO revision (entity, entity_id, member_id, at, before) VALUES (?, ?, ?, ?, ?)`, entity, id, me, db.Now(), string(b))
	return err
}

// ---- People ----

type personCredit struct {
	Role       string  `json:"role"`
	Character  string  `json:"character,omitempty"`
	Production summary `json:"production"`
}

type person struct {
	ID          int64          `json:"id"`
	Name        string         `json:"name"`
	Favorite    bool           `json:"favorite"`
	Note        string         `json:"note"`
	ExternalIDs []externalID   `json:"external_ids"`
	Credits     []personCredit `json:"credits"`
}

func (s *Server) getPerson(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	id, _ := pathID(r)
	p := person{ID: id, Credits: []personCredit{}}
	if err := s.db.QueryRow(`SELECT name FROM person WHERE id = ?`, id).Scan(&p.Name); err != nil {
		notFoundOr(w, err)
		return
	}
	p.Favorite, p.Note = personal(s.db, m.ID, "person", id)
	var err error
	if p.ExternalIDs, err = externalIDs(s.db, "person", id); err != nil {
		serverError(w, err)
		return
	}
	sums, err := summaries(s.db, m.ID, "WHERE p.id IN (SELECT production_id FROM credit WHERE person_id = ?)", id)
	if err != nil {
		serverError(w, err)
		return
	}
	byID := map[int64]summary{}
	for _, su := range sums {
		byID[su.ID] = su
	}
	rows, err := s.db.Query(`SELECT production_id, role, coalesce(character, '') FROM credit WHERE person_id = ? AND production_id IS NOT NULL`, id)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var c personCredit
		var pid int64
		if err := rows.Scan(&pid, &c.Role, &c.Character); err != nil {
			serverError(w, err)
			return
		}
		c.Production = byID[pid]
		p.Credits = append(p.Credits, c)
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) updatePerson(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	id, _ := pathID(r)
	var in struct{ Name string }
	if !decode(w, r, &in) {
		return
	}
	name := strings.Join(strings.Fields(in.Name), " ")
	if name == "" {
		writeError(w, http.StatusBadRequest, "Le nom est obligatoire.")
		return
	}
	err := s.inTx(func(tx *sql.Tx) error {
		var before personRef
		if err := tx.QueryRow(`SELECT id, name FROM person WHERE id = ?`, id).Scan(&before.ID, &before.Name); err != nil {
			return err
		}
		if err := saveRevision(tx, "person", id, m.ID, before); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE person SET name = ?, name_key = ? WHERE id = ?`, name, db.NameKey(name), id)
		return err
	})
	if err != nil {
		notFoundOr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"id": id})
}

// ---- Venues ----

type venue struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	City        string    `json:"city"`
	Address     string    `json:"address"`
	Favorite    bool      `json:"favorite"`
	Note        string    `json:"note"`
	Productions []summary `json:"productions"`
}

func (s *Server) getVenue(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	id, _ := pathID(r)
	v := venue{ID: id}
	if err := s.db.QueryRow(`SELECT name, coalesce(city, ''), coalesce(address, '') FROM venue WHERE id = ?`, id).
		Scan(&v.Name, &v.City, &v.Address); err != nil {
		notFoundOr(w, err)
		return
	}
	v.Favorite, v.Note = personal(s.db, m.ID, "venue", id)
	var err error
	v.Productions, err = summaries(s.db, m.ID, `
		JOIN (SELECT production_id, max(coalesce(ends_on, starts_on, '')) AS last FROM run WHERE venue_id = ? GROUP BY production_id) here
		  ON here.production_id = p.id
		ORDER BY here.last DESC`, id)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) updateVenue(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	id, _ := pathID(r)
	var in struct{ Name, City, Address string }
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		writeError(w, http.StatusBadRequest, "Le nom est obligatoire.")
		return
	}
	err := s.inTx(func(tx *sql.Tx) error {
		var before struct{ Name, City, Address sql.NullString }
		if err := tx.QueryRow(`SELECT name, city, address FROM venue WHERE id = ?`, id).Scan(&before.Name, &before.City, &before.Address); err != nil {
			return err
		}
		if err := saveRevision(tx, "venue", id, m.ID, map[string]string{"name": before.Name.String, "city": before.City.String, "address": before.Address.String}); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE venue SET name = ?, city = ?, address = ? WHERE id = ?`,
			strings.TrimSpace(in.Name), nullString(in.City), nullString(in.Address), id)
		return err
	})
	if err != nil {
		notFoundOr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"id": id})
}

// ---- Local search ----

// ftsQuery turns free text into an FTS5 query where every word is a prefix.
func ftsQuery(text string) string {
	var terms []string
	for _, w := range strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		terms = append(terms, `"`+w+`"*`)
	}
	return strings.Join(terms, " ")
}

type searchResult struct {
	Productions []summary   `json:"productions"`
	People      []personRef `json:"people"`
	Venues      []venueRef  `json:"venues"`
	Works       []workRef   `json:"works"`
}

// search queries the local catalogue. ?kind= limits it to one kind, for pickers.
func (s *Server) search(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	match := ftsQuery(r.URL.Query().Get("q"))
	kind := r.URL.Query().Get("kind")
	res := searchResult{Productions: []summary{}, People: []personRef{}, Venues: []venueRef{}, Works: []workRef{}}
	if match == "" {
		writeJSON(w, http.StatusOK, res)
		return
	}
	ids := func(entity string) string {
		return `(SELECT entity_id FROM search WHERE search MATCH ? AND entity = '` + entity + `' ORDER BY rank LIMIT 20)`
	}
	var err error
	if kind == "" || kind == "production" {
		// The member's own comments are searched too, for them only.
		comment := "%" + strings.TrimSpace(r.URL.Query().Get("q")) + "%"
		if res.Productions, err = summaries(s.db, m.ID, "WHERE p.id IN "+ids("production")+
			" OR p.id IN (SELECT production_id FROM outing WHERE member_id = @me AND comment LIKE ?) ORDER BY r.ends_on DESC LIMIT 30",
			match, comment); err != nil {
			serverError(w, err)
			return
		}
	}
	scan := func(entity, query string, add func(rows *sql.Rows) error) error {
		if kind != "" && kind != entity {
			return nil
		}
		rows, err := s.db.Query(query+` WHERE id IN `+ids(entity)+` ORDER BY 2`, match)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			if err := add(rows); err != nil {
				return err
			}
		}
		return rows.Err()
	}
	err = scan("person", `SELECT id, name FROM person`, func(rows *sql.Rows) error {
		var p personRef
		err := rows.Scan(&p.ID, &p.Name)
		res.People = append(res.People, p)
		return err
	})
	if err == nil {
		err = scan("venue", `SELECT id, name, coalesce(city, '') FROM venue`, func(rows *sql.Rows) error {
			var v venueRef
			err := rows.Scan(&v.ID, &v.Name, &v.City)
			res.Venues = append(res.Venues, v)
			return err
		})
	}
	if err == nil {
		err = scan("work", `SELECT id, title FROM work`, func(rows *sql.Rows) error {
			var wk workRef
			err := rows.Scan(&wk.ID, &wk.Title)
			res.Works = append(res.Works, wk)
			return err
		})
	}
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// roles is public: the frontend loads this constant list before sign-in.
func (s *Server) roles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, Roles)
}
