package api

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestExportAndDeleteAccount(t *testing.T) {
	e := newTestEnv(t)
	c := e.client("lucie")
	in := misanthrope()
	in["status"] = "want"
	var created struct{ ID int64 }
	c.do("POST", "/api/productions", in, http.StatusCreated, &created)
	var p production
	c.do("GET", fmt.Sprintf("/api/productions/%d", created.ID), nil, http.StatusOK, &p)
	c.do("PUT", fmt.Sprintf("/api/outings/%d", p.Outings[0].ID), map[string]any{"status": "seen", "run_id": p.Runs[0].ID, "date": "2026-12-05", "rating": 4, "comment": "Magnétique; vraiment."}, http.StatusOK, nil)
	c.do("PUT", fmt.Sprintf("/api/favorites/person/%d", p.Credits[1].Person.ID), nil, http.StatusNoContent, nil)
	c.do("PUT", fmt.Sprintf("/api/notes/venue/%d", p.Runs[0].Venue.ID), map[string]string{"body": "Places en haut : bof."}, http.StatusNoContent, nil)

	var ex Export
	c.do("GET", "/api/export.json", nil, http.StatusOK, &ex)
	if ex.Member.Login != "lucie" || len(ex.Outings) != 1 || ex.Outings[0].Production.Title != "Le Misanthrope" ||
		len(ex.Outings[0].Production.Credits) != 3 || len(ex.Favorites) != 1 || len(ex.Notes) != 1 || ex.Notes[0].Name != "Théâtre de la Cité" {
		t.Fatalf("export = %+v", ex)
	}

	resp, err := c.http.Get(e.srv.URL + "/api/export.csv")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\uFEFF")))
	r.Comma = ';'
	rows, err := r.ReadAll()
	if err != nil || len(rows) != 2 || rows[1][0] != "vu" || rows[1][4] != "Théâtre de la Cité" || rows[1][7] != "Magnétique; vraiment." || rows[1][8] != "Claire Vasseur" {
		t.Fatalf("csv = %q, %v", rows, err)
	}

	// Another member's export doesn't include Lucie's data.
	var other Export
	e.client("admin").do("GET", "/api/export.json", nil, http.StatusOK, &other)
	if len(other.Outings) != 0 || len(other.Notes) != 0 {
		t.Fatal("export leaked another member's data")
	}

	// Deletion needs the password, keeps the catalogue and removes the personal layer.
	c.do("DELETE", "/api/me", map[string]string{"password": "nope"}, http.StatusForbidden, nil)
	c.do("DELETE", "/api/me", map[string]string{"password": "lucie-pass"}, http.StatusNoContent, nil)
	c.do("GET", "/api/me", nil, http.StatusUnauthorized, nil)
	var n int
	e.db.QueryRow(`SELECT count(*) FROM outing`).Scan(&n)
	if n != 0 {
		t.Fatal("outings left behind")
	}
	e.db.QueryRow(`SELECT count(*) FROM production WHERE created_by IS NULL`).Scan(&n)
	if n != 1 {
		t.Fatal("catalogue contribution not kept")
	}

	// The only admin can't leave.
	e.client("admin").do("DELETE", "/api/me", map[string]string{"password": "admin-pass"}, http.StatusBadRequest, nil)
}
