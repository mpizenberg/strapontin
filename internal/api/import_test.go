package api

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// venueSite serves the extraction fixtures as if they were a theatre's website.
func venueSite(t *testing.T) *httptest.Server {
	var poster bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	img.Set(0, 0, color.RGBA{163, 34, 28, 255})
	png.Encode(&poster, img)
	mux := http.NewServeMux()
	for _, name := range []string{"tec-graph", "opengraph-only", "festik-list"} {
		body, err := os.ReadFile("../extract/testdata/" + name + ".html")
		if err != nil {
			t.Fatal(err)
		}
		mux.HandleFunc("/"+name, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(body)
		})
	}
	mux.HandleFunc("/poster.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(poster.Bytes())
	})
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/tec-graph", http.StatusFound) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestImport(t *testing.T) {
	e := newTestEnv(t)
	site := venueSite(t)
	c := e.client("lucie")

	// The catalogue already knows the venue and two people called Camille Rey.
	var existing struct{ ID int64 }
	c.do("POST", "/api/productions", map[string]any{
		"title": "Les Fourberies",
		"credits": []map[string]any{
			{"person": map[string]any{"name": "Camille Rey"}, "role": "performer"},
			{"person": map[string]any{"name": "Camille  REY"}, "role": "director"},
		},
		"runs": []map[string]any{{"venue": map[string]any{"name": "Théâtre du Pavé", "city": "Toulouse"}}},
	}, http.StatusCreated, &existing)

	var res importResult
	c.do("POST", "/api/import", map[string]string{"url": site.URL + "/tec-graph"}, http.StatusOK, &res)
	d := res.Draft
	if d == nil || d.Title != "Sganarelle ou le cocu imaginaire & L’Amour médecin" || d.SnapshotID == 0 || d.Source != "jsonld" {
		t.Fatalf("draft = %+v", d)
	}
	if len(d.Runs) != 1 || d.Runs[0].Venue.ID == 0 || d.Runs[0].StartsOn != "2026-11-18" || d.Runs[0].Guessed {
		t.Fatalf("runs = %+v", d.Runs)
	}

	// Saving links the snapshot and stores the poster.
	save := map[string]any{
		"title": d.Title, "company": d.Company, "url": d.URL, "status": "want",
		"snapshot_id": d.SnapshotID, "image": site.URL + "/poster.png",
		"runs": []map[string]any{{"venue": map[string]any{"id": d.Runs[0].Venue.ID}, "starts_on": d.Runs[0].StartsOn, "ends_on": d.Runs[0].EndsOn}},
	}
	var created struct{ ID int64 }
	c.do("POST", "/api/productions", save, http.StatusCreated, &created)
	var p production
	c.do("GET", fmt.Sprintf("/api/productions/%d", created.ID), nil, http.StatusOK, &p)
	if p.Poster == "" || p.URL != site.URL+"/tec-graph" {
		t.Fatalf("saved = %+v", p)
	}
	resp, err := c.http.Get(e.srv.URL + p.Poster)
	if err != nil || resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("poster: %v %v", resp, err)
	}
	resp.Body.Close()
	var linked int64
	e.db.QueryRow(`SELECT production_id FROM snapshot WHERE id = ?`, d.SnapshotID).Scan(&linked)
	if linked != created.ID {
		t.Fatal("snapshot not linked")
	}

	// The same page, directly or through a redirect, is a duplicate.
	c.do("POST", "/api/import", map[string]string{"url": site.URL + "/tec-graph"}, http.StatusOK, &res)
	if res.Duplicate == nil || res.Duplicate.ID != created.ID || res.Duplicate.Mine.Status != "want" {
		t.Fatalf("duplicate = %+v", res)
	}
	res = importResult{}
	c.do("POST", "/api/import", map[string]string{"url": site.URL + "/old"}, http.StatusOK, &res)
	if res.Duplicate == nil {
		t.Fatalf("redirect not detected as duplicate: %+v", res)
	}

	// Homonyms come back as candidates, without an id.
	res = importResult{}
	c.do("POST", "/api/import", map[string]string{"url": site.URL + "/festik-list"}, http.StatusOK, &res)
	var camille *draftCredit
	for i, cr := range res.Draft.Credits {
		if cr.Person.Name == "Camille Rey" {
			camille = &res.Draft.Credits[i]
		}
	}
	if camille == nil || camille.Person.ID != 0 || len(camille.Candidates) != 2 {
		t.Fatalf("homonyms = %+v", res.Draft.Credits)
	}

	// A page naming no venue gets the venue of earlier imports from the same site.
	res = importResult{}
	c.do("POST", "/api/import", map[string]string{"url": site.URL + "/opengraph-only"}, http.StatusOK, &res)
	if len(res.Draft.Runs) != 1 || !res.Draft.Runs[0].Guessed || res.Draft.Runs[0].Venue.Name != "Théâtre du Pavé" {
		t.Fatalf("guessed venue = %+v", res.Draft.Runs)
	}
	if res.Draft.Image != site.URL+"/assets/uploads/timidite.jpg" {
		t.Fatalf("image = %q", res.Draft.Image)
	}

	// Credits on the page text are read too.
	if len(res.Draft.Credits) != 5 || res.Draft.Credits[1].Role != "director" {
		t.Fatalf("text credits = %+v", res.Draft.Credits)
	}

	var pasted []draftCredit
	c.do("POST", "/api/credits/parse", map[string]string{"text": "Mise en scène : Camille Rey\nAvec Inès Bakari"}, http.StatusOK, &pasted)
	if len(pasted) != 2 || len(pasted[0].Candidates) != 2 || pasted[1].Person.ID != 0 || pasted[1].Role != "performer" {
		t.Fatalf("pasted credits = %+v", pasted)
	}

	c.do("POST", "/api/import", map[string]string{"url": site.URL + "/missing"}, http.StatusBadGateway, nil)
	c.do("POST", "/api/import", map[string]string{"url": "ftp://example.test/x"}, http.StatusBadRequest, nil)
	e.client("").do("POST", "/api/import", map[string]string{"url": site.URL + "/tec-graph"}, http.StatusUnauthorized, nil)
}
