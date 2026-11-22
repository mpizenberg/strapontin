package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/mpizenberg/strapontin/internal/auth"
	"github.com/mpizenberg/strapontin/internal/db"
)

// ---- Outings ----

type outing struct {
	ID           int64     `json:"id"`
	ProductionID int64     `json:"production_id"`
	RunID        *int64    `json:"run_id"`
	Venue        *venueRef `json:"venue,omitempty"` // venue of the run, when set
	Status       string    `json:"status"`
	Date         string    `json:"date,omitempty"`
	Rating       *int      `json:"rating,omitempty"`
	Comment      string    `json:"comment,omitempty"`
	CreatedAt    string    `json:"created_at"`
	UpdatedAt    string    `json:"updated_at"`
	Production   *summary  `json:"production,omitempty"` // only in lists
}

// outings lists the member's outings matching a condition on "o".
func outings(q queryer, me int64, where string, args ...any) ([]outing, error) {
	rows, err := q.Query(`
		SELECT o.id, o.production_id, o.run_id, v.id, v.name, v.city, o.status, o.date, o.rating, o.comment, o.created_at, o.updated_at
		FROM outing o LEFT JOIN run r ON r.id = o.run_id LEFT JOIN venue v ON v.id = r.venue_id
		WHERE o.member_id = @me AND `+where+`
		ORDER BY o.date IS NULL, o.date DESC, o.id DESC`, append([]any{sql.Named("me", me)}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []outing{}
	for rows.Next() {
		var o outing
		var runID, venueID, rating sql.NullInt64
		var venueName, city, date, comment sql.NullString
		if err := rows.Scan(&o.ID, &o.ProductionID, &runID, &venueID, &venueName, &city, &o.Status, &date, &rating, &comment, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		if runID.Valid {
			o.RunID = &runID.Int64
		}
		if venueID.Valid {
			o.Venue = &venueRef{ID: venueID.Int64, Name: venueName.String, City: city.String}
		}
		if rating.Valid {
			r := int(rating.Int64)
			o.Rating = &r
		}
		o.Date, o.Comment = date.String, comment.String
		list = append(list, o)
	}
	return list, rows.Err()
}

type outingInput struct {
	ProductionID int64  `json:"production_id"`
	RunID        *int64 `json:"run_id"`
	Status       string `json:"status"`
	Date         string `json:"date"`
	Rating       *int   `json:"rating"`
	Comment      string `json:"comment"`
}

func (in *outingInput) validate(q queryer) error {
	if in.Status != "want" && in.Status != "seen" {
		return errInvalid("Statut inconnu.")
	}
	if !validDate(in.Date) {
		return errInvalid("Date invalide.")
	}
	if in.Rating != nil && (*in.Rating < 1 || *in.Rating > 5) {
		return errInvalid("La note va de 1 à 5.")
	}
	if in.Status == "want" {
		// A wish has no viewing details.
		in.RunID, in.Date, in.Rating, in.Comment = nil, "", nil, ""
	}
	if in.RunID != nil {
		var n int
		q.QueryRow(`SELECT count(*) FROM run WHERE id = ? AND production_id = ?`, *in.RunID, in.ProductionID).Scan(&n)
		if n == 0 {
			return errInvalid("Cette représentation n'appartient pas au spectacle.")
		}
	}
	return nil
}

func insertOuting(q queryer, me int64, in outingInput) (int64, error) {
	if err := q.QueryRow(`SELECT id FROM production WHERE id = ?`, in.ProductionID).Scan(&in.ProductionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errInvalid("Spectacle introuvable.")
		}
		return 0, err
	}
	if err := in.validate(q); err != nil {
		return 0, err
	}
	now := db.Now()
	res, err := q.Exec(`INSERT INTO outing (member_id, production_id, run_id, status, date, rating, comment, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		me, in.ProductionID, in.RunID, in.Status, nullString(in.Date), in.Rating, nullString(in.Comment), now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// listOutings serves À voir (?status=want) and the journal (?status=seen).
func (s *Server) listOutings(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	status := r.URL.Query().Get("status")
	where, args := "1", []any{}
	if status != "" {
		where, args = "o.status = ?", []any{status}
	}
	list, err := outings(s.db, m.ID, where, args...)
	if err != nil {
		serverError(w, err)
		return
	}
	sums, err := summaries(s.db, m.ID, "WHERE p.id IN (SELECT production_id FROM outing WHERE member_id = @me)")
	if err != nil {
		serverError(w, err)
		return
	}
	byID := map[int64]*summary{}
	for i := range sums {
		byID[sums[i].ID] = &sums[i]
	}
	for i := range list {
		list[i].Production = byID[list[i].ProductionID]
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createOuting(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	var in outingInput
	if !decode(w, r, &in) {
		return
	}
	id, err := insertOuting(s.db, m.ID, in)
	if err != nil {
		writeSaveError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (s *Server) updateOuting(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "Introuvable.")
		return
	}
	var in outingInput
	if !decode(w, r, &in) {
		return
	}
	if err := s.db.QueryRow(`SELECT production_id FROM outing WHERE id = ? AND member_id = ?`, id, m.ID).Scan(&in.ProductionID); err != nil {
		notFoundOr(w, err)
		return
	}
	if err := in.validate(s.db); err != nil {
		writeSaveError(w, err)
		return
	}
	_, err := s.db.Exec(`UPDATE outing SET run_id = ?, status = ?, date = ?, rating = ?, comment = ?, updated_at = ? WHERE id = ? AND member_id = ?`,
		in.RunID, in.Status, nullString(in.Date), in.Rating, nullString(in.Comment), db.Now(), id, m.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"id": id})
}

func (s *Server) deleteOuting(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	id, _ := pathID(r)
	if _, err := s.db.Exec(`DELETE FROM outing WHERE id = ? AND member_id = ?`, id, m.ID); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Favourites and notes ----

var personalEntities = map[string]string{"person": "person", "work": "work", "venue": "venue", "production": "production"}

// entityParam validates {entity}/{id}; favourites don't apply to productions.
func entityParam(w http.ResponseWriter, r *http.Request, allowProduction bool) (string, int64, bool) {
	entity, ok := personalEntities[r.PathValue("entity")]
	id, okID := pathID(r)
	if !ok || !okID || (entity == "production" && !allowProduction) {
		writeError(w, http.StatusNotFound, "Introuvable.")
		return "", 0, false
	}
	return entity, id, true
}

func (s *Server) putFavorite(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	entity, id, ok := entityParam(w, r, false)
	if !ok {
		return
	}
	if !s.exists(w, entity, id) {
		return
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO favorite (member_id, entity, entity_id, created_at) VALUES (?, ?, ?, ?)`,
		m.ID, entity, id, db.Now()); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteFavorite(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	entity, id, ok := entityParam(w, r, false)
	if !ok {
		return
	}
	if _, err := s.db.Exec(`DELETE FROM favorite WHERE member_id = ? AND entity = ? AND entity_id = ?`, m.ID, entity, id); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// exists answers 404 unless the catalogue row exists. entity must come from
// personalEntities, since it is spliced into the query.
func (s *Server) exists(w http.ResponseWriter, entity string, id int64) bool {
	var n int
	s.db.QueryRow(`SELECT count(*) FROM `+entity+` WHERE id = ?`, id).Scan(&n)
	if n == 0 {
		writeError(w, http.StatusNotFound, "Introuvable.")
		return false
	}
	return true
}

type favorite struct {
	Entity      string   `json:"entity"`
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	City        string   `json:"city,omitempty"`
	Roles       []string `json:"roles,omitempty"` // a person's roles, most frequent first
	Productions int      `json:"productions"`
}

func (s *Server) listFavorites(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	rows, err := s.db.Query(`
		SELECT f.entity, f.entity_id,
		  coalesce(pe.name, v.name, wk.title, ''), coalesce(v.city, ''),
		  CASE f.entity
		    WHEN 'person' THEN (SELECT group_concat(role) FROM (SELECT role FROM credit WHERE person_id = f.entity_id
		                         GROUP BY role ORDER BY count(*) DESC, min(id)))
		  END,
		  CASE f.entity
		    WHEN 'person' THEN (SELECT count(DISTINCT production_id) FROM credit WHERE person_id = f.entity_id)
		    WHEN 'venue' THEN (SELECT count(DISTINCT production_id) FROM run WHERE venue_id = f.entity_id)
		    WHEN 'work' THEN (SELECT count(*) FROM production WHERE work_id = f.entity_id)
		  END
		FROM favorite f
		LEFT JOIN person pe ON f.entity = 'person' AND pe.id = f.entity_id
		LEFT JOIN venue v ON f.entity = 'venue' AND v.id = f.entity_id
		LEFT JOIN work wk ON f.entity = 'work' AND wk.id = f.entity_id
		WHERE f.member_id = ?
		ORDER BY f.entity = 'person' DESC, 3`, m.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()
	list := []favorite{}
	for rows.Next() {
		var f favorite
		var roles sql.NullString
		if err := rows.Scan(&f.Entity, &f.ID, &f.Name, &f.City, &roles, &f.Productions); err != nil {
			serverError(w, err)
			return
		}
		if roles.String != "" {
			f.Roles = strings.Split(roles.String, ",")
		}
		list = append(list, f)
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) putNote(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	entity, id, ok := entityParam(w, r, true)
	if !ok {
		return
	}
	var in struct{ Body string }
	if !decode(w, r, &in) {
		return
	}
	if !s.exists(w, entity, id) {
		return
	}
	var err error
	if strings.TrimSpace(in.Body) == "" {
		_, err = s.db.Exec(`DELETE FROM note WHERE member_id = ? AND entity = ? AND entity_id = ?`, m.ID, entity, id)
	} else {
		_, err = s.db.Exec(`INSERT INTO note (member_id, entity, entity_id, body, updated_at) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (member_id, entity, entity_id) DO UPDATE SET body = excluded.body, updated_at = excluded.updated_at`,
			m.ID, entity, id, in.Body, db.Now())
	}
	if err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// personal returns the member's favourite flag and note for a catalogue row.
func personal(q queryer, me int64, entity string, id int64) (favorite bool, note string) {
	q.QueryRow(`SELECT EXISTS (SELECT 1 FROM favorite WHERE member_id = ? AND entity = ? AND entity_id = ?),
		coalesce((SELECT body FROM note WHERE member_id = ? AND entity = ? AND entity_id = ?), '')`,
		me, entity, id, me, entity, id).Scan(&favorite, &note)
	return
}

// ---- Me ----

func (s *Server) me(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	out := struct {
		*auth.Member
		CreatedAt string `json:"created_at"`
		Seen      int    `json:"seen"`
		Wanted    int    `json:"wanted"`
	}{Member: m}
	err := s.db.QueryRow(`SELECT created_at,
		(SELECT count(DISTINCT production_id) FROM outing WHERE member_id = m.id AND status = 'seen'),
		(SELECT count(*) FROM outing WHERE member_id = m.id AND status = 'want')
		FROM member m WHERE id = ?`, m.ID).Scan(&out.CreatedAt, &out.Seen, &out.Wanted)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
