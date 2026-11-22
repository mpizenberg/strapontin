package extract

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// item is a microdata item: its type and properties, each a string or a nested item.
type item struct {
	typ   string
	props map[string][]any
}

func readItem(n *html.Node) item {
	it := item{typ: attr(n, "itemtype"), props: map[string][]any{}}
	var walk func(*html.Node)
	walk = func(c *html.Node) {
		for ; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode {
				continue
			}
			_, scope := hasAttr(c, "itemscope")
			if prop := attr(c, "itemprop"); prop != "" {
				var v any
				if scope {
					v = readItem(c)
				} else {
					v = propValue(c)
				}
				for _, name := range strings.Fields(prop) {
					it.props[name] = append(it.props[name], v)
				}
			}
			if !scope {
				walk(c.FirstChild)
			}
		}
	}
	walk(n.FirstChild)
	return it
}

func propValue(n *html.Node) string {
	switch n.DataAtom {
	case atom.Meta:
		return attr(n, "content")
	case atom.Img, atom.Audio, atom.Video, atom.Source:
		return attr(n, "src")
	case atom.A, atom.Link, atom.Area:
		return attr(n, "href")
	case atom.Time:
		if v := attr(n, "datetime"); v != "" {
			return v
		}
	case atom.Data, atom.Meter:
		return attr(n, "value")
	}
	if v := attr(n, "content"); v != "" {
		return v
	}
	return textContent(n)
}

func (it item) str(name string) string {
	for _, v := range it.props[name] {
		switch v := v.(type) {
		case string:
			return clean(v)
		case item:
			return v.str("name")
		}
	}
	return ""
}

func (it item) sub(name string) (item, bool) {
	for _, v := range it.props[name] {
		if s, ok := v.(item); ok {
			return s, true
		}
	}
	return item{}, false
}

// applyMicrodata fills the draft from the first schema.org Event item.
func applyMicrodata(d *Draft, items []item) bool {
	for _, it := range items {
		if !strings.HasSuffix(it.typ, "Event") {
			continue
		}
		set(&d.Title, it.str("name"))
		set(&d.Description, it.str("description"))
		set(&d.Image, it.str("image"))
		set(&d.StartsOn, date(it.str("startDate")))
		set(&d.EndsOn, date(it.str("endDate")))
		if loc, ok := it.sub("location"); ok {
			set(&d.VenueName, loc.str("name"))
			if addr, ok := loc.sub("address"); ok {
				set(&d.VenueCity, addr.str("addressLocality"))
				set(&d.VenueAddress, addr.str("streetAddress"))
			}
		} else {
			set(&d.VenueName, it.str("location"))
		}
		for _, role := range []struct{ prop, key string }{{"performer", "performer"}, {"actor", "performer"}, {"director", "director"}} {
			for _, v := range it.props[role.prop] {
				switch v := v.(type) {
				case string:
					addCredit(d, Credit{Name: clean(v), Role: role.key})
				case item:
					addCredit(d, Credit{Name: v.str("name"), Role: role.key})
				}
			}
		}
		return true
	}
	return false
}
