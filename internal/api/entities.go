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
	Homonyms    []homonym      `json:"homonyms"`
}

// homonym is another person with the same name key, offered for merging.
type homonym struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Roles       []string `json:"roles"`
	Productions int      `json:"productions"`
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
	if p.Homonyms, err = homonyms(s.db, id); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func homonyms(q queryer, id int64) ([]homonym, error) {
	rows, err := q.Query(`
		SELECT pe.id, pe.name, coalesce((SELECT group_concat(DISTINCT role) FROM credit WHERE person_id = pe.id), ''),
		  (SELECT count(DISTINCT production_id) FROM credit WHERE person_id = pe.id)
		FROM person pe
		WHERE pe.name_key = (SELECT name_key FROM person WHERE id = ?) AND pe.id <> ?
		ORDER BY pe.id`, id, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hs := []homonym{}
	for rows.Next() {
		var h homonym
		var roles string
		if err := rows.Scan(&h.ID, &h.Name, &roles, &h.Productions); err != nil {
			return nil, err
		}
		h.Roles = strings.FieldsFunc(roles, func(r rune) bool { return r == ',' })
		hs = append(hs, h)
	}
	return hs, rows.Err()
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

// mergePerson moves a homonym's credits, favorites, notes and external ids to
// this person and deletes the homonym. Different names don't merge: a member
// renames first, so a wrong id can't fold two people together.
func (s *Server) mergePerson(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	id, _ := pathID(r)
	var in struct{ From int64 }
	if !decode(w, r, &in) {
		return
	}
	if in.From == id {
		writeError(w, http.StatusBadRequest, "Une fiche ne se fusionne pas avec elle-même.")
		return
	}
	err := s.inTx(func(tx *sql.Tx) error {
		var key, fromKey, fromName string
		if err := tx.QueryRow(`SELECT name_key FROM person WHERE id = ?`, id).Scan(&key); err != nil {
			return err
		}
		if err := tx.QueryRow(`SELECT name, name_key FROM person WHERE id = ?`, in.From).Scan(&fromName, &fromKey); err != nil {
			return err
		}
		if key != fromKey {
			return errInvalid("Seules des fiches au même nom se fusionnent. Renommez d'abord l'une des deux.")
		}
		var distinct bool
		if err := tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM external_id a JOIN external_id b ON a.scheme = b.scheme
			WHERE a.entity = 'person' AND a.entity_id = ? AND b.entity = 'person' AND b.entity_id = ?)`, id, in.From).Scan(&distinct); err != nil {
			return err
		}
		if distinct {
			return errInvalid("Ces fiches sont liées à deux entrées Wikidata différentes : ce sont deux personnes.")
		}
		if err := saveRevision(tx, "person", in.From, m.ID, map[string]any{"id": in.From, "name": fromName, "merged_into": id}); err != nil {
			return err
		}
		for _, q := range []string{
			`UPDATE credit SET person_id = @into WHERE person_id = @from`,
			// Both people credited in the same role on the same production.
			`DELETE FROM credit WHERE person_id = @into AND id NOT IN
			   (SELECT min(id) FROM credit WHERE person_id = @into GROUP BY production_id, work_id, role, coalesce(character, ''))`,
			`INSERT OR IGNORE INTO favorite (member_id, entity, entity_id, created_at)
			   SELECT member_id, 'person', @into, created_at FROM favorite WHERE entity = 'person' AND entity_id = @from`,
			`INSERT INTO note (member_id, entity, entity_id, body, updated_at)
			   SELECT member_id, 'person', @into, body, updated_at FROM note WHERE entity = 'person' AND entity_id = @from
			   ON CONFLICT (member_id, entity, entity_id) DO UPDATE
			   SET body = note.body || char(10) || char(10) || excluded.body, updated_at = max(note.updated_at, excluded.updated_at)`,
			`DELETE FROM favorite WHERE entity = 'person' AND entity_id = @from`,
			`DELETE FROM note WHERE entity = 'person' AND entity_id = @from`,
			`UPDATE external_id SET entity_id = @into WHERE entity = 'person' AND entity_id = @from`,
			`DELETE FROM person WHERE id = @from`,
		} {
			if _, err := tx.Exec(q, sql.Named("into", id), sql.Named("from", in.From)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		writeSaveError(w, err)
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
