package sources

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/mpizenberg/strapontin/internal/db"
)

// The keyless endpoint used here is OpenAgenda's internal web API: it is not
// documented and may change. Everything that depends on its shape stays in
// this file; if it breaks, the search reports the source as unavailable and
// URL import still works.

// toulouseAgendas are Toulouse Métropole (which aggregates the city, cultural
// centres and suburbs) and the city of Toulouse.
var toulouseAgendas = []string{"50522407", "42448083"}

type oaEvent struct {
	UID      json.RawMessage   `json:"uid"` // a number or a string
	Slug     string            `json:"slug"`
	Title    map[string]string `json:"title"`
	Location struct {
		Name string `json:"name"`
		City string `json:"city"`
	} `json:"location"`
	FirstTiming struct {
		Begin string `json:"begin"`
	} `json:"firstTiming"`
	LastTiming struct {
		End string `json:"end"`
	} `json:"lastTiming"`
	OriginAgenda struct {
		Slug string `json:"slug"`
	} `json:"originAgenda"`
}

// OpenAgenda searches the Toulouse agendas by title, past and future events alike.
func (c *Client) OpenAgenda(ctx context.Context, q string) ([]Candidate, error) {
	seen := map[string]bool{}
	out := []Candidate{}
	for _, agenda := range toulouseAgendas {
		var res struct {
			Events []oaEvent `json:"events"`
		}
		err := c.getJSON(ctx, c.OpenAgendaURL, "/api/agendas/"+agenda+"/events",
			url.Values{"search": {q}, "size": {"20"}}, &res)
		if err != nil {
			return nil, err
		}
		for _, e := range res.Events {
			title := e.Title["fr"]
			if title == "" {
				for _, t := range e.Title {
					title = t
					break
				}
			}
			uid := strings.Trim(string(e.UID), `"`)
			// The search also matches venue names ("théâtre" returns a whole
			// opera house): keep events whose title holds the query.
			if title == "" || uid == "" || seen[uid] || !containsWords(title, q) {
				continue
			}
			seen[uid] = true
			link := "https://openagenda.com/agendas/" + agenda + "/events/" + uid
			if e.OriginAgenda.Slug != "" && e.Slug != "" {
				link = "https://openagenda.com/" + e.OriginAgenda.Slug + "/events/" + e.Slug
			}
			out = append(out, Candidate{
				Title: title, Venue: e.Location.Name, City: e.Location.City,
				StartsOn: day(e.FirstTiming.Begin), EndsOn: day(e.LastTiming.End),
				URL: link, Source: "openagenda",
			})
		}
	}
	return out, nil
}

// containsWords reports whether every word of q appears in s, ignoring case and accents.
func containsWords(s, q string) bool {
	key := " " + db.NameKey(s) + " "
	for _, w := range strings.Fields(db.NameKey(q)) {
		if !strings.Contains(key, w) {
			return false
		}
	}
	return true
}

func day(s string) string {
	if len(s) >= 10 && s[4] == '-' {
		return s[:10]
	}
	return ""
}
