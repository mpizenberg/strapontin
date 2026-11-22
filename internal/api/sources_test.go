package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mpizenberg/strapontin/internal/sources"
)

// fakeSources answers like the three outside services, in the shapes the
// clients expect.
func fakeSources(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agendas/{uid}/events", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("uid") == "42448083" {
			// The second agenda repeats an event and adds a venue match.
			fmt.Fprint(w, `{"events":[{"uid":7402852,"slug":"maman","title":{"fr":"Maman Dragonne"}},
				{"uid":"999","title":{"fr":"Saison du Théâtre du Capitole"}}]}`)
			return
		}
		fmt.Fprint(w, `{"events":[{"uid":7402852,"slug":"theatre-maman-dragonne","title":{"fr":"Théâtre : Maman Dragonne"},
			"location":{"name":"Le Phare","city":"Tournefeuille"},"firstTiming":{"begin":"2026-11-01T15:00:00+01:00"},
			"lastTiming":{"end":"2027-01-10T16:00:00+01:00"},"originAgenda":{"slug":"le-phare-tournefeuille"}}]}`)
	})
	mux.HandleFunc("/wp-json/wp/v2/agenda", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"link":"https://tourisme.example/agenda/je-gere/","title":{"rendered":"Je g&egrave;re"}},
			{"link":"https://tourisme.example/agenda/je-gere-2/","title":{"rendered":"Je g&egrave;re"}}]`)
	})
	mux.HandleFunc("/w/api.php", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") != "wbsearchentities" || r.URL.Query().Get("language") != "fr" {
			http.Error(w, "bad request", 400)
			return
		}
		fmt.Fprint(w, `{"search":[{"id":"Q687","label":"Molière","description":"dramaturge français"}]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestSources(t *testing.T) {
	e := newTestEnv(t)
	fake := fakeSources(t)
	e.api.sources.OpenAgendaURL, e.api.sources.TourismeURL, e.api.sources.WikidataURL = fake.URL, fake.URL, fake.URL
	c := e.client("lucie")

	var oa []sources.Candidate
	c.do("GET", "/api/sources/search?source=openagenda&q=maman+dragonne", nil, http.StatusOK, &oa)
	if len(oa) != 1 || oa[0].URL != "https://openagenda.com/le-phare-tournefeuille/events/theatre-maman-dragonne" ||
		oa[0].StartsOn != "2026-11-01" || oa[0].EndsOn != "2027-01-10" || oa[0].City != "Tournefeuille" {
		t.Fatalf("openagenda = %+v", oa)
	}
	var tour []sources.Candidate
	c.do("GET", "/api/sources/search?source=tourisme&q=je+gere", nil, http.StatusOK, &tour)
	if len(tour) != 1 || tour[0].Title != "Je gère" {
		t.Fatalf("tourisme = %+v", tour)
	}
	c.do("GET", "/api/sources/search?source=nope&q=x", nil, http.StatusBadRequest, nil)

	// A source that is down is reported, not hidden.
	e.api.sources.TourismeURL = "http://127.0.0.1:1"
	c.do("GET", "/api/sources/search?source=tourisme&q=je+gere", nil, http.StatusBadGateway, nil)

	var ents []sources.Entity
	c.do("GET", "/api/wikidata/search?q=moli", nil, http.StatusOK, &ents)
	if len(ents) != 1 || ents[0].ID != "Q687" {
		t.Fatalf("wikidata = %+v", ents)
	}

	var created struct{ ID int64 }
	c.do("POST", "/api/productions", misanthrope(), http.StatusCreated, &created)
	var p production
	c.do("GET", fmt.Sprintf("/api/productions/%d", created.ID), nil, http.StatusOK, &p)
	moliere, claire := p.Credits[0].Person.ID, p.Credits[1].Person.ID
	c.do("PUT", fmt.Sprintf("/api/people/%d/wikidata", moliere), map[string]string{"id": "Q687"}, http.StatusNoContent, nil)
	c.do("PUT", fmt.Sprintf("/api/people/%d/wikidata", claire), map[string]string{"id": "Q687"}, http.StatusBadRequest, nil)
	c.do("PUT", fmt.Sprintf("/api/people/%d/wikidata", claire), map[string]string{"id": "javascript:1"}, http.StatusBadRequest, nil)
	var pe person
	c.do("GET", fmt.Sprintf("/api/people/%d", moliere), nil, http.StatusOK, &pe)
	if len(pe.ExternalIDs) != 1 || pe.ExternalIDs[0].Value != "Q687" {
		t.Fatalf("external ids = %+v", pe.ExternalIDs)
	}
	c.do("PUT", fmt.Sprintf("/api/people/%d/wikidata", moliere), map[string]string{"id": ""}, http.StatusNoContent, nil)
	c.do("GET", fmt.Sprintf("/api/people/%d", moliere), nil, http.StatusOK, &pe)
	if len(pe.ExternalIDs) != 0 {
		t.Fatal("link not removed")
	}
}
