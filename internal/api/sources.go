package api

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/mpizenberg/strapontin/internal/auth"
	"github.com/mpizenberg/strapontin/internal/sources"
)

// searchSources queries one outside agenda. The client asks each source
// separately, so one failing source doesn't hide the others.
func (s *Server) searchSources(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	search, ok := map[string]func(context.Context, string) ([]sources.Candidate, error){
		"openagenda": s.sources.OpenAgenda,
		"tourisme":   s.sources.Tourisme,
	}[r.URL.Query().Get("source")]
	if !ok {
		writeError(w, http.StatusBadRequest, "Source inconnue.")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) < 2 {
		writeJSON(w, http.StatusOK, []sources.Candidate{})
		return
	}
	list, err := search(r.Context(), q)
	auth.LogEvent(s.db, &m.ID, "source_search")
	if err != nil {
		log.Printf("source %s: %v", r.URL.Query().Get("source"), err)
		writeError(w, http.StatusBadGateway, "Source indisponible pour le moment.")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) searchWikidata(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, []sources.Entity{})
		return
	}
	list, err := s.sources.Wikidata(r.Context(), q)
	if err != nil {
		log.Printf("wikidata: %v", err)
		writeError(w, http.StatusBadGateway, "Wikidata ne répond pas pour le moment.")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

var qid = regexp.MustCompile(`^Q[1-9][0-9]*$`)

// linkWikidata sets or clears (empty id) a person's Wikidata item.
func (s *Server) linkWikidata(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	id, _ := pathID(r)
	var in struct{ ID string }
	if !decode(w, r, &in) {
		return
	}
	if in.ID != "" && !qid.MatchString(in.ID) {
		writeError(w, http.StatusBadRequest, "Identifiant Wikidata invalide.")
		return
	}
	if !s.exists(w, "person", id) {
		return
	}
	err := s.inTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM external_id WHERE entity = 'person' AND entity_id = ? AND scheme = 'wikidata'`, id); err != nil {
			return err
		}
		if in.ID == "" {
			return nil
		}
		var other int64
		switch err := tx.QueryRow(`SELECT entity_id FROM external_id WHERE scheme = 'wikidata' AND value = ?`, in.ID).Scan(&other); {
		case err == nil:
			return errInvalid("Cette entrée Wikidata est déjà liée à une autre fiche : ce sont peut-être deux fiches pour la même personne.")
		case !errors.Is(err, sql.ErrNoRows):
			return err
		}
		_, err := tx.Exec(`INSERT INTO external_id (entity, entity_id, scheme, value) VALUES ('person', ?, 'wikidata', ?)`, id, in.ID)
		return err
	})
	if err != nil {
		writeSaveError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
