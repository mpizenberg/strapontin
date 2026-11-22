package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mpizenberg/strapontin/internal/auth"
	"github.com/mpizenberg/strapontin/internal/db"
	"github.com/mpizenberg/strapontin/internal/extract"
)

// snapshotReuse is how long a fetched page is reused instead of fetched again.
const snapshotReuse = 24 * time.Hour

// candidate is an existing person whose name matches a credit.
type candidate struct {
	ID    int64    `json:"id"`
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
}

// draftCredit is a proposed credit; the person has an id when exactly one
// existing person matches, and candidates when several do.
type draftCredit struct {
	Person     personRef   `json:"person"`
	Role       string      `json:"role"`
	Character  string      `json:"character"`
	Candidates []candidate `json:"candidates,omitempty"`
}

type draftRun struct {
	Venue    venueRef `json:"venue"`
	StartsOn string   `json:"starts_on"`
	EndsOn   string   `json:"ends_on"`
	// Guessed is set when the page named no venue and the venue was guessed from the site.
	Guessed bool `json:"guessed,omitempty"`
}

// importDraft has the shape of productionInput, plus what the review form needs.
type importDraft struct {
	SnapshotID  int64         `json:"snapshot_id"`
	URL         string        `json:"url"`
	Image       string        `json:"image,omitempty"`
	Source      string        `json:"source"`
	Title       string        `json:"title"`
	Work        *workRef      `json:"work"`
	Company     string        `json:"company"`
	Description string        `json:"description"`
	Credits     []draftCredit `json:"credits"`
	Runs        []draftRun    `json:"runs"`
}

type importResult struct {
	Duplicate *summary     `json:"duplicate,omitempty"`
	Draft     *importDraft `json:"draft,omitempty"`
}

func (s *Server) importURL(w http.ResponseWriter, r *http.Request, m *auth.Member) {
	var in struct{ URL string }
	if !decode(w, r, &in) {
		return
	}
	u, err := extract.ParseURL(in.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Cette adresse n'est pas valide.")
		return
	}
	if dup, err := s.productionByURL(m.ID, u.String()); err != nil {
		serverError(w, err)
		return
	} else if dup != nil {
		writeJSON(w, http.StatusOK, importResult{Duplicate: dup})
		return
	}

	snap, err := s.snapshot(r.Context(), u.String(), m.ID)
	if err != nil {
		log.Printf("import %s: %v", u, err)
		writeError(w, http.StatusBadGateway, "Impossible de lire cette page. Vérifiez l'adresse, ou saisissez le spectacle à la main.")
		return
	}
	if snap.status >= 400 {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Le site répond « erreur %d » : la page a peut-être été retirée.", snap.status))
		return
	}
	// A redirect may land on a page already in the catalogue.
	if snap.url != u.String() {
		if dup, err := s.productionByURL(m.ID, snap.url); err == nil && dup != nil {
			writeJSON(w, http.StatusOK, importResult{Duplicate: dup})
			return
		}
	}
	d, err := extract.Extract(snap.body, snap.contentType, snap.url)
	if err != nil {
		serverError(w, err)
		return
	}
	extracted, _ := json.Marshal(d)
	s.db.Exec(`UPDATE snapshot SET extracted = ?, extractor_version = ? WHERE id = ?`, string(extracted), extract.Version, snap.id)

	draft, err := s.match(d, snap.url)
	if err != nil {
		serverError(w, err)
		return
	}
	draft.SnapshotID = snap.id
	writeJSON(w, http.StatusOK, importResult{Draft: draft})
}

// productionByURL returns the production already imported from a URL, if any.
func (s *Server) productionByURL(me int64, u string) (*summary, error) {
	list, err := summaries(s.db, me, `WHERE p.id = (SELECT entity_id FROM external_id WHERE entity = 'production' AND scheme = 'url' AND value = ?)`, u)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0], nil
}

type storedSnapshot struct {
	id          int64
	url         string
	status      int
	contentType string
	body        []byte
}

// snapshot returns a recent snapshot of the URL, or fetches and stores a new one.
func (s *Server) snapshot(ctx context.Context, u string, me int64) (*storedSnapshot, error) {
	snap := storedSnapshot{url: u}
	var gz []byte
	var contentType sql.NullString
	err := s.db.QueryRow(`SELECT id, http_status, content_type, body FROM snapshot WHERE url = ? AND fetched_at > ? ORDER BY id DESC LIMIT 1`,
		u, time.Now().UTC().Add(-snapshotReuse).Format(time.RFC3339)).Scan(&snap.id, &snap.status, &contentType, &gz)
	if err == nil {
		snap.contentType = contentType.String
		snap.body, err = gunzip(gz)
		return &snap, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	resp, err := s.fetcher.Get(ctx, u)
	auth.LogEvent(s.db, &me, "fetch")
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write(resp.Body)
	zw.Close()
	sum := sha256.Sum256(resp.Body)
	// The snapshot is stored under the requested URL so the 24 h reuse finds it;
	// resp.URL only differs after a redirect.
	res, err := s.db.Exec(`INSERT INTO snapshot (url, fetched_at, fetched_by, http_status, content_type, body, sha256, extractor_version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, u, db.Now(), me, resp.Status, resp.ContentType, buf.Bytes(), hex.EncodeToString(sum[:]), extract.Version)
	if err != nil {
		return nil, err
	}
	snap.id, _ = res.LastInsertId()
	snap.url, snap.status, snap.contentType, snap.body = resp.URL, resp.Status, resp.ContentType, resp.Body
	return &snap, nil
}

func gunzip(b []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(zr)
}

// match turns an extracted draft into a form draft, linking names to
// existing people, venues and works.
func (s *Server) match(d *extract.Draft, pageURL string) (*importDraft, error) {
	out := &importDraft{
		URL: pageURL, Image: d.Image, Source: d.Source,
		Title: d.Title, Company: d.Company, Description: d.Description,
		Credits: []draftCredit{}, Runs: []draftRun{},
	}
	for _, c := range d.Credits {
		dc := draftCredit{Person: personRef{Name: c.Name}, Role: c.Role, Character: c.Character}
		cands, err := s.peopleNamed(c.Name)
		if err != nil {
			return nil, err
		}
		if len(cands) == 1 {
			dc.Person = personRef{ID: cands[0].ID, Name: cands[0].Name}
		} else if len(cands) > 1 {
			dc.Candidates = cands
		}
		out.Credits = append(out.Credits, dc)
	}

	if d.WorkTitle != "" {
		out.Work = &workRef{Title: d.WorkTitle}
	}
	if id, title, err := s.workTitled(firstNonEmpty(d.WorkTitle, d.Title)); err != nil {
		return nil, err
	} else if id != 0 {
		out.Work = &workRef{ID: id, Title: title}
	}

	run := draftRun{Venue: venueRef{Name: d.VenueName, City: d.VenueCity}, StartsOn: d.StartsOn, EndsOn: d.EndsOn}
	if d.VenueName != "" {
		if v, err := s.venueNamed(d.VenueName); err != nil {
			return nil, err
		} else if v != nil {
			run.Venue = *v
		}
	} else if v, err := s.venueOfSite(pageURL); err != nil {
		return nil, err
	} else if v != nil {
		run.Venue, run.Guessed = *v, true
	}
	if run.Venue.Name != "" || run.StartsOn != "" || run.EndsOn != "" {
		out.Runs = append(out.Runs, run)
	}
	return out, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// peopleNamed lists the people whose name key matches, with their roles to tell homonyms apart.
func (s *Server) peopleNamed(name string) ([]candidate, error) {
	rows, err := s.db.Query(`
		SELECT p.id, p.name, coalesce((SELECT group_concat(role) FROM (SELECT DISTINCT role FROM credit WHERE person_id = p.id)), '')
		FROM person p WHERE p.name_key = ? ORDER BY p.id`, db.NameKey(name))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []candidate
	for rows.Next() {
		var c candidate
		var roles string
		if err := rows.Scan(&c.ID, &c.Name, &roles); err != nil {
			return nil, err
		}
		if roles != "" {
			c.Roles = strings.Split(roles, ",")
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

// venueNamed finds a venue whose name has the same key. Venue names are few,
// so they are compared in Go rather than indexed.
func (s *Server) venueNamed(name string) (*venueRef, error) {
	rows, err := s.db.Query(`SELECT id, name, coalesce(city, '') FROM venue`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	key := db.NameKey(name)
	for rows.Next() {
		var v venueRef
		if err := rows.Scan(&v.ID, &v.Name, &v.City); err != nil {
			return nil, err
		}
		if db.NameKey(v.Name) == key {
			return &v, nil
		}
	}
	return nil, rows.Err()
}

// venueOfSite guesses the venue of a page that names none: the venue most
// used by productions imported from the same site.
func (s *Server) venueOfSite(pageURL string) (*venueRef, error) {
	u, err := url.Parse(pageURL)
	if err != nil || u.Host == "" {
		return nil, nil
	}
	var v venueRef
	err = s.db.QueryRow(`
		SELECT v.id, v.name, coalesce(v.city, '') FROM external_id e
		JOIN run r ON r.production_id = e.entity_id JOIN venue v ON v.id = r.venue_id
		WHERE e.entity = 'production' AND e.scheme = 'url' AND (e.value LIKE ? OR e.value LIKE ?)
		GROUP BY v.id ORDER BY count(*) DESC LIMIT 1`,
		"http://"+u.Host+"/%", "https://"+u.Host+"/%").Scan(&v.ID, &v.Name, &v.City)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &v, err
}

func (s *Server) workTitled(title string) (int64, string, error) {
	if title == "" {
		return 0, "", nil
	}
	rows, err := s.db.Query(`SELECT id, title FROM work`)
	if err != nil {
		return 0, "", err
	}
	defer rows.Close()
	key := db.NameKey(title)
	for rows.Next() {
		var id int64
		var t string
		if err := rows.Scan(&id, &t); err != nil {
			return 0, "", err
		}
		if db.NameKey(t) == key {
			return id, t, nil
		}
	}
	return 0, "", rows.Err()
}

// posterTypes are the image formats kept as posters. SVG is excluded: it can carry scripts.
var posterTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true, "image/gif": true, "image/avif": true}

// savePoster downloads an image for a production. Failures only cost the poster.
func (s *Server) savePoster(ctx context.Context, productionID int64, imageURL string) {
	if imageURL == "" {
		return
	}
	resp, err := s.fetcher.Get(ctx, imageURL)
	if err != nil || resp.Status != http.StatusOK {
		log.Printf("poster %s: %v", imageURL, err)
		return
	}
	ct := strings.ToLower(strings.TrimSpace(strings.SplitN(resp.ContentType, ";", 2)[0]))
	if !posterTypes[ct] {
		ct = http.DetectContentType(resp.Body)
	}
	if !posterTypes[ct] {
		log.Printf("poster %s: unsupported type %q", imageURL, ct)
		return
	}
	if _, err := s.db.Exec(`INSERT INTO image (production_id, source_url, content_type, bytes, fetched_at) VALUES (?, ?, ?, ?, ?)`,
		productionID, imageURL, ct, resp.Body, db.Now()); err != nil {
		log.Printf("poster %s: %v", imageURL, err)
	}
}
