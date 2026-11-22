// Package sources searches outside agendas and Wikidata, only when a member asks.
package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/mpizenberg/strapontin/internal/extract"
)

// Candidate is a show found in an outside agenda. Picking it imports URL.
type Candidate struct {
	Title    string `json:"title"`
	Venue    string `json:"venue,omitempty"`
	City     string `json:"city,omitempty"`
	StartsOn string `json:"starts_on,omitempty"`
	EndsOn   string `json:"ends_on,omitempty"`
	URL      string `json:"url"`
	Source   string `json:"source"`
}

// Client holds the base URL of each source, so tests can point them at a local server.
type Client struct {
	fetcher       *extract.Fetcher
	OpenAgendaURL string
	TourismeURL   string
	WikidataURL   string
}

func New(f *extract.Fetcher) *Client {
	return &Client{
		fetcher:       f,
		OpenAgendaURL: "https://openagenda.com",
		TourismeURL:   "https://www.toulouse-tourisme.com",
		WikidataURL:   "https://www.wikidata.org",
	}
}

func (c *Client) getJSON(ctx context.Context, base, path string, query url.Values, out any) error {
	resp, err := c.fetcher.Get(ctx, base+path+"?"+query.Encode())
	if err != nil {
		return err
	}
	if resp.Status != 200 {
		return fmt.Errorf("%s answered %d", base, resp.Status)
	}
	return json.Unmarshal(resp.Body, out)
}
