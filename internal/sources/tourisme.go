package sources

import (
	"context"
	"html"
	"net/url"
)

// Tourisme searches the Toulouse tourism office agenda (WordPress REST API).
// It covers venues absent from OpenAgenda, but finished events disappear.
func (c *Client) Tourisme(ctx context.Context, q string) ([]Candidate, error) {
	var res []struct {
		Link  string `json:"link"`
		Title struct {
			Rendered string `json:"rendered"`
		} `json:"title"`
	}
	err := c.getJSON(ctx, c.TourismeURL, "/wp-json/wp/v2/agenda",
		url.Values{"search": {q}, "per_page": {"10"}, "_fields": {"link,title"}}, &res)
	if err != nil {
		return nil, err
	}
	out := []Candidate{}
	seen := map[string]bool{}
	for _, e := range res {
		title := html.UnescapeString(e.Title.Rendered)
		// The office sometimes has two entries for the same show.
		if e.Link == "" || title == "" || seen[title] {
			continue
		}
		seen[title] = true
		out = append(out, Candidate{Title: title, URL: e.Link, Source: "tourisme"})
	}
	return out, nil
}
