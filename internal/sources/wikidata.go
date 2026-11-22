package sources

import (
	"context"
	"net/url"
)

// Entity is a Wikidata item proposed when linking a person.
type Entity struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// Wikidata searches items by label, in French.
func (c *Client) Wikidata(ctx context.Context, q string) ([]Entity, error) {
	var res struct {
		Search []Entity `json:"search"`
	}
	err := c.getJSON(ctx, c.WikidataURL, "/w/api.php", url.Values{
		"action": {"wbsearchentities"}, "search": {q}, "language": {"fr"}, "uselang": {"fr"},
		"type": {"item"}, "limit": {"8"}, "format": {"json"},
	}, &res)
	if err != nil {
		return nil, err
	}
	if res.Search == nil {
		res.Search = []Entity{}
	}
	return res.Search, nil
}
