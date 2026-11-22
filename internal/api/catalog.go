package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mpizenberg/strapontin/internal/auth"
	"github.com/mpizenberg/strapontin/internal/db"
)

// Roles is the closed list of credit roles, in display order, with gender-neutral French labels.
var Roles = []struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}{
	{"author", "texte"},
	{"adaptation", "adaptation"},
	{"translation", "traduction"},
	{"director", "mise en scène"},
	{"assistant_director", "assistanat à la mise en scène"},
	{"performer", "interprète"},
	{"dramaturgy", "dramaturgie"},
	{"set_design", "scénographie"},
	{"lighting", "lumière"},
	{"sound", "son"},
	{"music", "musique"},
	{"costumes", "costumes"},
	{"choreography", "chorégraphie"},
	{"video", "vidéo"},
}

func validRole(key string) bool {
	for _, r := range Roles {
		if r.Key == key {
			return true
		}
	}
	return false
}

// queryer is satisfied by both *sql.DB and *sql.Tx.
type queryer interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// errInvalid carries a message for the member; handlers answer 400 with it.
type errInvalid string

func (e errInvalid) Error() string { return string(e) }

// writeSaveError answers 400 for validation errors and 500 otherwise.
func writeSaveError(w http.ResponseWriter, err error) {
	var inv errInvalid
	if errors.As(err, &inv) {
		writeError(w, http.StatusBadRequest, string(inv))
		return
	}
	notFoundOr(w, err)
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func validDate(s string) bool {
	if s == "" {
		return true
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// nullString stores empty strings as NULL.
func nullString(s string) any {
	if s = strings.TrimSpace(s); s == "" {
		return nil
	}
	return s
}

// ---- Shapes shared by several responses ----

type personRef struct {
	ID   int64  `json:"id,omitempty"`
	Name string `json:"name"`
}

type workRef struct {
	ID    int64  `json:"id,omitempty"`
	Title string `json:"title"`
}

type venueRef struct {
	ID   int64  `json:"id,omitempty"`
	Name string `json:"name"`
	City string `json:"city,omitempty"`
}

// mine is the member's own relation to a production, embedded in catalogue
// responses and computed per request.
type mine struct {
	Status string `json:"status"`
	Rating *int   `json:"rating,omitempty"`
}

// summary is how a production appears in lists.
type summary struct {
	ID       int64     `json:"id"`
	Title    string    `json:"title"`
	Poster   string    `json:"poster,omitempty"`
	Venue    *venueRef `json:"venue,omitempty"`
	StartsOn string    `json:"starts_on,omitempty"`
	EndsOn   string    `json:"ends_on,omitempty"`
	Mine     *mine     `json:"mine,omitempty"`
}

// summarySQL selects summaries; @me is the member id. The run shown is the
// one ending last, which is the current or upcoming one for a touring show.
const summarySQL = `
SELECT p.id, p.title,
  (SELECT max(i.id) FROM image i WHERE i.production_id = p.id),
  v.id, v.name, v.city, r.starts_on, r.ends_on,
  (SELECT CASE WHEN max(o.status = 'seen') THEN 'seen' WHEN count(*) > 0 THEN 'want' END
     FROM outing o WHERE o.production_id = p.id AND o.member_id = @me),
  (SELECT max(o.rating) FROM outing o WHERE o.production_id = p.id AND o.member_id = @me AND o.status = 'seen')
FROM production p
LEFT JOIN run r ON r.id = (SELECT r2.id FROM run r2 WHERE r2.production_id = p.id
                           ORDER BY r2.ends_on IS NULL, r2.ends_on DESC, r2.id DESC LIMIT 1)
LEFT JOIN venue v ON v.id = r.venue_id
`

func scanSummary(sc interface{ Scan(...any) error }, s *summary) error {
	var poster, venueID sql.NullInt64
	var venueName, city, starts, ends, status sql.NullString
	var rating sql.NullInt64
	if err := sc.Scan(&s.ID, &s.Title, &poster, &venueID, &venueName, &city, &starts, &ends, &status, &rating); err != nil {
		return err
	}
	if poster.Valid {
		s.Poster = fmt.Sprintf("/api/images/%d", poster.Int64)
	}
	if venueID.Valid {
		s.Venue = &venueRef{ID: venueID.Int64, Name: venueName.String, City: city.String}
	}
	s.StartsOn, s.EndsOn = starts.String, ends.String
	if status.Valid {
		s.Mine = &mine{Status: status.String}
		if rating.Valid {
			r := int(rating.Int64)
			s.Mine.Rating = &r
		}
	}
	return nil
}

// summaries runs summarySQL with a trailing clause (WHERE, ORDER BY...).
func summaries(q queryer, me int64, clause string, args ...any) ([]summary, error) {
	rows, err := q.Query(summarySQL+clause, append([]any{sql.Named("me", me)}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []summary{}
	for rows.Next() {
		var s summary
		if err := scanSummary(rows, &s); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

// ---- Production ----

type credit struct {
	ID        int64     `json:"id"`
	Person    personRef `json:"person"`
	Role      string    `json:"role"`
	Character string    `json:"character,omitempty"`
	Favorite  bool      `json:"favorite"`
	Seen      int       `json:"seen"` // other productions with this person the member has seen
}

type run struct {
	ID       int64    `json:"id"`
	Venue    venueRef `json:"venue"`
	StartsOn string   `json:"starts_on,omitempty"`
	EndsOn   string   `json:"ends_on,omitempty"`
}

type production struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	Work        *workRef  `json:"work"`
	Company     string    `json:"company,omitempty"`
	Description string    `json:"description,omitempty"`
	URL         string    `json:"url,omitempty"`
	Poster      string    `json:"poster,omitempty"`
	Credits     []credit  `json:"credits"`
	Runs        []run     `json:"runs"`
	Revisions   int       `json:"revisions"`
	CreatedAt   string    `json:"created_at"`
	Outings     []outing  `json:"outings"`
	Others      []summary `json:"others"` // other stagings of the same work
}

// loadProduction reads a production with its credits and runs. With me > 0
// it also fills the member's outings, favourites and seen counts.
func loadProduction(q queryer, id, me int64) (*production, error) {
	p := production{ID: id, Credits: []credit{}, Runs: []run{}, Outings: []outing{}, Others: []summary{}}
	var workID sql.NullInt64
	var workTitle, company, description, url sql.NullString
	var poster sql.NullInt64
	err := q.QueryRow(`
		SELECT p.title, p.work_id, w.title, p.company, p.description, p.created_at,
		  (SELECT value FROM external_id WHERE entity = 'production' AND entity_id = p.id AND scheme = 'url' LIMIT 1),
		  (SELECT max(id) FROM image WHERE production_id = p.id),
		  (SELECT count(*) FROM revision WHERE entity = 'production' AND entity_id = p.id)
		FROM production p LEFT JOIN work w ON w.id = p.work_id WHERE p.id = ?`, id).
		Scan(&p.Title, &workID, &workTitle, &company, &description, &p.CreatedAt, &url, &poster, &p.Revisions)
	if err != nil {
		return nil, err
	}
	if workID.Valid {
		p.Work = &workRef{ID: workID.Int64, Title: workTitle.String}
	}
	p.Company, p.Description, p.URL = company.String, description.String, url.String
	if poster.Valid {
		p.Poster = fmt.Sprintf("/api/images/%d", poster.Int64)
	}

	rows, err := q.Query(`
		SELECT c.id, c.person_id, pe.name, c.role, coalesce(c.character, ''),
		  EXISTS (SELECT 1 FROM favorite f WHERE f.member_id = @me AND f.entity = 'person' AND f.entity_id = c.person_id),
		  (SELECT count(DISTINCT c2.production_id) FROM credit c2
		     JOIN outing o ON o.production_id = c2.production_id AND o.member_id = @me AND o.status = 'seen'
		     WHERE c2.person_id = c.person_id AND c2.production_id <> c.production_id)
		FROM credit c JOIN person pe ON pe.id = c.person_id
		WHERE c.production_id = @id ORDER BY c.position, c.id`, sql.Named("me", me), sql.Named("id", id))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c credit
		if err := rows.Scan(&c.ID, &c.Person.ID, &c.Person.Name, &c.Role, &c.Character, &c.Favorite, &c.Seen); err != nil {
			rows.Close()
			return nil, err
		}
		p.Credits = append(p.Credits, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = q.Query(`
		SELECT r.id, v.id, v.name, coalesce(v.city, ''), coalesce(r.starts_on, ''), coalesce(r.ends_on, '')
		FROM run r JOIN venue v ON v.id = r.venue_id
		WHERE r.production_id = ? ORDER BY r.starts_on IS NULL, r.starts_on, r.id`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r run
		if err := rows.Scan(&r.ID, &r.Venue.ID, &r.Venue.Name, &r.Venue.City, &r.StartsOn, &r.EndsOn); err != nil {
			rows.Close()
			return nil, err
		}
		p.Runs = append(p.Runs, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if me == 0 {
		return &p, nil
	}
	if p.Outings, err = outings(q, me, "o.production_id = ?", id); err != nil {
		return nil, err
	}
	if p.Work != nil {
		if p.Others, err = summaries(q, me, "WHERE p.work_id = ? AND p.id <> ? ORDER BY r.ends_on DESC", p.Work.ID, id); err != nil {
			return nil, err
		}
	}
	return &p, nil
}

// productionInput is what the production form sends, for both creation and
// edits. A reference without an id creates a new person, work or venue.
type productionInput struct {
	Title       string   `json:"title"`
	Work        *workRef `json:"work"`
	Company     string   `json:"company"`
	Description string   `json:"description"`
	URL         string   `json:"url"`
	Credits     []struct {
		Person    personRef `json:"person"`
		Role      string    `json:"role"`
		Character string    `json:"character"`
	} `json:"credits"`
	Runs []struct {
		ID       int64    `json:"id"`
		Venue    venueRef `json:"venue"`
		StartsOn string   `json:"starts_on"`
		EndsOn   string   `json:"ends_on"`
	} `json:"runs"`
}

func (in *productionInput) validate() error {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return errInvalid("Le titre est obligatoire.")
	}
	for _, c := range in.Credits {
		if !validRole(c.Role) {
			return errInvalid("Rôle inconnu : " + c.Role)
		}
		if c.Person.ID == 0 && strings.TrimSpace(c.Person.Name) == "" {
			return errInvalid("Un crédit n'a pas de nom.")
		}
	}
	for _, r := range in.Runs {
		if r.Venue.ID == 0 && strings.TrimSpace(r.Venue.Name) == "" {
			return errInvalid("Une représentation n'a pas de lieu.")
		}
		if !validDate(r.StartsOn) || !validDate(r.EndsOn) {
			return errInvalid("Date invalide.")
		}
	}
	return nil
}

func ensurePerson(tx queryer, ref personRef, me int64) (int64, error) {
	if ref.ID != 0 {
		return ref.ID, tx.QueryRow(`SELECT id FROM person WHERE id = ?`, ref.ID).Scan(&ref.ID)
	}
	name := strings.Join(strings.Fields(ref.Name), " ")
	res, err := tx.Exec(`INSERT INTO person (name, name_key, created_by, created_at) VALUES (?, ?, ?, ?)`,
		name, db.NameKey(name), me, db.Now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func ensureVenue(tx queryer, ref venueRef, me int64) (int64, error) {
	if ref.ID != 0 {
		return ref.ID, tx.QueryRow(`SELECT id FROM venue WHERE id = ?`, ref.ID).Scan(&ref.ID)
	}
	res, err := tx.Exec(`INSERT INTO venue (name, city, created_by, created_at) VALUES (?, ?, ?, ?)`,
		strings.TrimSpace(ref.Name), nullString(ref.City), me, db.Now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func ensureWork(tx queryer, ref *workRef, me int64) (any, error) {
	if ref == nil || (ref.ID == 0 && strings.TrimSpace(ref.Title) == "") {
		return nil, nil
	}
	if ref.ID != 0 {
		return ref.ID, tx.QueryRow(`SELECT id FROM work WHERE id = ?`, ref.ID).Scan(&ref.ID)
	}
	res, err := tx.Exec(`INSERT INTO work (title, created_by, created_at) VALUES (?, ?, ?)`, strings.TrimSpace(ref.Title), me, db.Now())
	if err != nil {
		return nil, err
	}
	return res.LastInsertId()
}

// saveProduction creates (id == 0) or replaces a production with its credits
// and runs. Runs keep their ids so outings stay attached; an edit stores the
// previous state as a revision.
func saveProduction(tx queryer, id int64, in *productionInput, me int64) (int64, error) {
	if err := in.validate(); err != nil {
		return 0, err
	}
	workID, err := ensureWork(tx, in.Work, me)
	if err != nil {
		return 0, err
	}
	if id == 0 {
		res, err := tx.Exec(`INSERT INTO production (title, work_id, company, description, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			in.Title, workID, nullString(in.Company), nullString(in.Description), me, db.Now())
		if err != nil {
			return 0, err
		}
		if id, err = res.LastInsertId(); err != nil {
			return 0, err
		}
	} else {
		before, err := loadProduction(tx, id, 0)
		if err != nil {
			return 0, err
		}
		snapshot, _ := json.Marshal(before)
		if _, err := tx.Exec(`INSERT INTO revision (entity, entity_id, member_id, at, before) VALUES ('production', ?, ?, ?, ?)`,
			id, me, db.Now(), string(snapshot)); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`UPDATE production SET title = ?, work_id = ?, company = ?, description = ? WHERE id = ?`,
			in.Title, workID, nullString(in.Company), nullString(in.Description), id); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`DELETE FROM credit WHERE production_id = ?`, id); err != nil {
			return 0, err
		}
	}

	for i, c := range in.Credits {
		personID, err := ensurePerson(tx, c.Person, me)
		if err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`INSERT INTO credit (production_id, person_id, role, character, position) VALUES (?, ?, ?, ?, ?)`,
			id, personID, c.Role, nullString(c.Character), i); err != nil {
			return 0, err
		}
	}

	kept := []string{"0"}
	for _, r := range in.Runs {
		venueID, err := ensureVenue(tx, r.Venue, me)
		if err != nil {
			return 0, err
		}
		runID := r.ID
		if runID != 0 {
			res, err := tx.Exec(`UPDATE run SET venue_id = ?, starts_on = ?, ends_on = ? WHERE id = ? AND production_id = ?`,
				venueID, nullString(r.StartsOn), nullString(r.EndsOn), runID, id)
			if err != nil {
				return 0, err
			}
			if n, _ := res.RowsAffected(); n == 0 {
				runID = 0
			}
		}
		if runID == 0 {
			res, err := tx.Exec(`INSERT INTO run (production_id, venue_id, starts_on, ends_on) VALUES (?, ?, ?, ?)`,
				id, venueID, nullString(r.StartsOn), nullString(r.EndsOn))
			if err != nil {
				return 0, err
			}
			runID, _ = res.LastInsertId()
		}
		kept = append(kept, strconv.FormatInt(runID, 10))
	}
	if _, err := tx.Exec(`DELETE FROM run WHERE production_id = ? AND id NOT IN (`+strings.Join(kept, ",")+`)`, id); err != nil {
		return 0, err
	}

	if url := strings.TrimSpace(in.URL); url != "" {
		var owner int64
		err := tx.QueryRow(`SELECT entity_id FROM external_id WHERE scheme = 'url' AND value = ? AND entity = 'production'`, url).Scan(&owner)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if _, err := tx.Exec(`INSERT INTO external_id (entity, entity_id, scheme, value) VALUES ('production', ?, 'url', ?)`, id, url); err != nil {
				return 0, err
			}
		case err != nil:
			return 0, err
		case owner != id:
			return 0, errInvalid("Cette adresse est déjà liée à un autre spectacle.")
		}
	}
	return id, nil
}

// inTx runs f in a transaction and commits when it succeeds.
func (s *Server) inTx(f func(tx *sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if err := f(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Server) getProduction(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "Introuvable.")
		return
	}
	p, err := loadProduction(s.db, id, m.ID)
	if err != nil {
		notFoundOr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// createProduction saves a new production, and the member's first outing
// when a status is given. An imported draft also links its snapshot and
// downloads its poster.
func (s *Server) createProduction(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	var in struct {
		productionInput
		Status     string `json:"status"`
		SnapshotID int64  `json:"snapshot_id"`
		Image      string `json:"image"`
	}
	if !decode(w, r, &in) {
		return
	}
	var id int64
	err := s.inTx(func(tx *sql.Tx) error {
		var err error
		if id, err = saveProduction(tx, 0, &in.productionInput, m.ID); err != nil {
			return err
		}
		if in.Status == "" {
			return nil
		}
		_, err = insertOuting(tx, m.ID, outingInput{ProductionID: id, Status: in.Status})
		return err
	})
	if err != nil {
		writeSaveError(w, err)
		return
	}
	if in.SnapshotID != 0 {
		s.db.Exec(`UPDATE snapshot SET production_id = ? WHERE id = ? AND production_id IS NULL`, id, in.SnapshotID)
		auth.LogEvent(s.db, &m.ID, "import")
	}
	s.savePoster(r.Context(), id, in.Image)
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (s *Server) updateProduction(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "Introuvable.")
		return
	}
	var in productionInput
	if !decode(w, r, &in) {
		return
	}
	err := s.inTx(func(tx *sql.Tx) error {
		_, err := saveProduction(tx, id, &in, m.ID)
		return err
	})
	if err != nil {
		writeSaveError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"id": id})
}

func (s *Server) getImage(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	var contentType string
	var body []byte
	if err := s.db.QueryRow(`SELECT content_type, bytes FROM image WHERE id = ?`, id).Scan(&contentType, &body); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Write(body)
}
