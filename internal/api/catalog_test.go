package api

import (
	"fmt"
	"net/http"
	"testing"
)

func misanthrope() map[string]any {
	return map[string]any{
		"title":   "Le Misanthrope",
		"work":    map[string]any{"title": "Le Misanthrope"},
		"company": "Compagnie des Lustres",
		"url":     "https://theatre.example/misanthrope",
		"credits": []map[string]any{
			{"person": map[string]any{"name": "Molière"}, "role": "author"},
			{"person": map[string]any{"name": "Claire Vasseur"}, "role": "director"},
			{"person": map[string]any{"name": "Inès Bakari"}, "role": "performer", "character": "Célimène"},
		},
		"runs": []map[string]any{
			{"venue": map[string]any{"name": "Théâtre de la Cité", "city": "Toulouse"}, "starts_on": "2026-12-02", "ends_on": "2026-12-12"},
		},
	}
}

func TestProductionLifecycle(t *testing.T) {
	e := newTestEnv(t)
	c := e.client("lucie")

	in := misanthrope()
	in["status"] = "want"
	var created struct{ ID int64 }
	c.do("POST", "/api/productions", in, http.StatusCreated, &created)

	var p production
	c.do("GET", fmt.Sprintf("/api/productions/%d", created.ID), nil, http.StatusOK, &p)
	if p.Title != "Le Misanthrope" || p.Work == nil || len(p.Credits) != 3 || len(p.Runs) != 1 || p.URL == "" {
		t.Fatalf("production = %+v", p)
	}
	if len(p.Outings) != 1 || p.Outings[0].Status != "want" {
		t.Fatalf("outings = %+v", p.Outings)
	}
	if p.Credits[2].Character != "Célimène" || p.Credits[0].Role != "author" {
		t.Fatalf("credits out of order: %+v", p.Credits)
	}

	// The wish becomes a viewing at the run.
	runID := p.Runs[0].ID
	c.do("PUT", fmt.Sprintf("/api/outings/%d", p.Outings[0].ID),
		map[string]any{"status": "seen", "run_id": runID, "date": "2026-12-05", "rating": 4, "comment": "Magnétique."}, http.StatusOK, nil)

	// Editing keeps the run id, reuses existing people by id and writes a revision.
	edit := misanthrope()
	edit["credits"] = []map[string]any{
		{"person": map[string]any{"id": p.Credits[1].Person.ID}, "role": "director"},
	}
	edit["runs"] = []map[string]any{
		{"id": runID, "venue": map[string]any{"id": p.Runs[0].Venue.ID}, "starts_on": "2026-12-02", "ends_on": "2026-12-13"},
	}
	c.do("PUT", fmt.Sprintf("/api/productions/%d", p.ID), edit, http.StatusOK, nil)
	c.do("GET", fmt.Sprintf("/api/productions/%d", p.ID), nil, http.StatusOK, &p)
	if len(p.Credits) != 1 || p.Runs[0].ID != runID || p.Runs[0].EndsOn != "2026-12-13" || p.Revisions != 1 {
		t.Fatalf("after edit = %+v", p)
	}
	if p.Outings[0].Venue == nil || p.Outings[0].Venue.Name != "Théâtre de la Cité" || *p.Outings[0].Rating != 4 {
		t.Fatalf("outing after edit = %+v", p.Outings[0])
	}

	// Journal lists it with its summary.
	var journal []outing
	c.do("GET", "/api/outings?status=seen", nil, http.StatusOK, &journal)
	if len(journal) != 1 || journal[0].Production == nil || journal[0].Production.Mine.Status != "seen" {
		t.Fatalf("journal = %+v", journal)
	}

	// Another member sees the catalogue but not Lucie's outing, and can't touch it.
	other := e.client("admin")
	var seenByOther production
	other.do("GET", fmt.Sprintf("/api/productions/%d", p.ID), nil, http.StatusOK, &seenByOther)
	if len(seenByOther.Outings) != 0 {
		t.Fatal("outings leaked to another member")
	}
	other.do("PUT", fmt.Sprintf("/api/outings/%d", p.Outings[0].ID), map[string]any{"status": "want"}, http.StatusNotFound, nil)
	other.do("DELETE", fmt.Sprintf("/api/outings/%d", p.Outings[0].ID), nil, http.StatusNoContent, nil)
	c.do("GET", "/api/outings?status=seen", nil, http.StatusOK, &journal)
	if len(journal) != 1 {
		t.Fatal("another member deleted the outing")
	}

	// The same URL can't describe two productions.
	dup := misanthrope()
	c.do("POST", "/api/productions", dup, http.StatusBadRequest, nil)
}

func TestProductionValidation(t *testing.T) {
	e := newTestEnv(t)
	c := e.client("lucie")
	c.do("POST", "/api/productions", map[string]any{"title": " "}, http.StatusBadRequest, nil)
	c.do("POST", "/api/productions", map[string]any{"title": "X", "credits": []map[string]any{
		{"person": map[string]any{"name": "A"}, "role": "auteur"},
	}}, http.StatusBadRequest, nil)
	c.do("POST", "/api/productions", map[string]any{"title": "X", "runs": []map[string]any{
		{"venue": map[string]any{"name": "V"}, "ends_on": "12/12/2026"},
	}}, http.StatusBadRequest, nil)
	c.do("POST", "/api/outings", map[string]any{"production_id": 999, "status": "want"}, http.StatusBadRequest, nil)
	c.do("GET", "/api/productions/999", nil, http.StatusNotFound, nil)
}

func TestPersonFavoritesNotesSearch(t *testing.T) {
	e := newTestEnv(t)
	c := e.client("lucie")
	var created struct{ ID int64 }
	c.do("POST", "/api/productions", misanthrope(), http.StatusCreated, &created)
	var p production
	c.do("GET", fmt.Sprintf("/api/productions/%d", created.ID), nil, http.StatusOK, &p)
	claire := p.Credits[1].Person.ID
	venueID := p.Runs[0].Venue.ID

	c.do("PUT", fmt.Sprintf("/api/favorites/person/%d", claire), nil, http.StatusNoContent, nil)
	c.do("PUT", fmt.Sprintf("/api/favorites/venue/%d", venueID), nil, http.StatusNoContent, nil)
	c.do("PUT", "/api/favorites/person/999", nil, http.StatusNotFound, nil)
	c.do("PUT", fmt.Sprintf("/api/notes/person/%d", claire), map[string]string{"body": "Découverte avec La Cerisaie."}, http.StatusNoContent, nil)

	var pe person
	c.do("GET", fmt.Sprintf("/api/people/%d", claire), nil, http.StatusOK, &pe)
	if !pe.Favorite || pe.Note == "" || len(pe.Credits) != 1 || pe.Credits[0].Production.Venue.Name != "Théâtre de la Cité" {
		t.Fatalf("person = %+v", pe)
	}

	var favs []favorite
	c.do("GET", "/api/favorites", nil, http.StatusOK, &favs)
	if len(favs) != 2 || favs[0].Name != "Claire Vasseur" || favs[0].Roles[0] != "director" || favs[1].City != "Toulouse" {
		t.Fatalf("favorites = %+v", favs)
	}

	// Notes and favourites are private.
	other := e.client("admin")
	other.do("GET", fmt.Sprintf("/api/people/%d", claire), nil, http.StatusOK, &pe)
	if pe.Favorite || pe.Note != "" {
		t.Fatal("personal data leaked")
	}

	var res searchResult
	c.do("GET", "/api/search?q=vass", nil, http.StatusOK, &res)
	if len(res.People) != 1 || res.People[0].Name != "Claire Vasseur" {
		t.Fatalf("search people = %+v", res)
	}
	c.do("GET", "/api/search?q=misanthr", nil, http.StatusOK, &res)
	if len(res.Productions) != 1 || len(res.Works) != 1 {
		t.Fatalf("search productions = %+v", res)
	}
	c.do("POST", "/api/outings", map[string]any{"production_id": p.ID, "status": "maybe"}, http.StatusBadRequest, nil)
	c.do("POST", "/api/outings", map[string]any{"production_id": p.ID, "status": "seen", "comment": "Une scène du sonnet inoubliable"}, http.StatusCreated, nil)
	c.do("GET", "/api/search?q=sonnet", nil, http.StatusOK, &res)
	if len(res.Productions) != 1 {
		t.Fatalf("own comment not searched: %+v", res)
	}
	other.do("GET", "/api/search?q=sonnet", nil, http.StatusOK, &res)
	if len(res.Productions) != 0 {
		t.Fatal("comment search leaked to another member")
	}
	c.do("GET", "/api/search?q=theatre%20cite&kind=venue", nil, http.StatusOK, &res)
	if len(res.Venues) != 1 || len(res.Productions) != 0 {
		t.Fatalf("search venues = %+v", res)
	}

	c.do("PUT", fmt.Sprintf("/api/people/%d", claire), map[string]string{"name": "Claire  Vasseur-Roy"}, http.StatusOK, nil)
	c.do("GET", "/api/search?q=roy", nil, http.StatusOK, &res)
	if len(res.People) != 1 || res.People[0].Name != "Claire Vasseur-Roy" {
		t.Fatalf("renamed person not found: %+v", res)
	}

	var v venue
	c.do("GET", fmt.Sprintf("/api/venues/%d", venueID), nil, http.StatusOK, &v)
	if !v.Favorite || len(v.Productions) != 1 {
		t.Fatalf("venue = %+v", v)
	}
}
