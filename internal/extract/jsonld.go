package extract

import (
	"encoding/json"
	"strings"
)

// node is one JSON-LD object.
type node = map[string]any

// applyJSONLD fills the draft from the page's JSON-LD and reports whether it
// described an event or a creative work. Event pages often list one node per
// date: the run spans from the first start to the last end.
func applyJSONLD(d *Draft, scripts []string) bool {
	var nodes []node
	byID := map[string]node{}
	var collect func(v any)
	collect = func(v any) {
		switch v := v.(type) {
		case []any:
			for _, e := range v {
				collect(e)
			}
		case node:
			if id, ok := v["@id"].(string); ok {
				byID[id] = v
			}
			if g, ok := v["@graph"]; ok {
				collect(g)
			}
			nodes = append(nodes, v)
		}
	}
	for _, s := range scripts {
		var v any
		// Some sites leave raw newlines inside strings; tolerate them.
		if json.Unmarshal([]byte(strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(s)), &v) == nil {
			collect(v)
		}
	}
	resolve := func(v any) any {
		if m, ok := v.(node); ok && len(m) == 1 {
			if id, ok := m["@id"].(string); ok && byID[id] != nil {
				return byID[id]
			}
		}
		return v
	}

	var events, works []node
	for _, n := range nodes {
		switch {
		case hasType(n, "Event"):
			events = append(events, n)
		case hasType(n, "CreativeWork", "Play", "TheaterPlay", "Book"):
			works = append(works, n)
		}
	}
	main := append(events, works...)
	if len(main) == 0 {
		return false
	}
	first := main[0]
	set(&d.Title, text(first["name"]))
	set(&d.Description, htmlText(text(first["description"])))
	set(&d.Image, imageURL(resolve(first["image"]), resolve))

	for _, e := range events {
		if text(e["name"]) != text(first["name"]) {
			continue
		}
		if s := date(text(e["startDate"])); s != "" && (d.StartsOn == "" || s < d.StartsOn) {
			d.StartsOn = s
		}
		end := date(text(e["endDate"]))
		if end == "" {
			end = date(text(e["startDate"]))
		}
		if end != "" && end > d.EndsOn {
			d.EndsOn = end
		}
		if loc, ok := resolve(one(e["location"])).(node); ok {
			set(&d.VenueName, text(loc["name"]))
			switch addr := resolve(loc["address"]).(type) {
			case node:
				set(&d.VenueCity, text(addr["addressLocality"]))
				set(&d.VenueAddress, text(addr["streetAddress"]))
			case string:
				set(&d.VenueAddress, addr)
			}
		}
		addPeople(d, e["performer"], "performer", resolve)
		addPeople(d, e["actor"], "performer", resolve)
		addPeople(d, e["director"], "director", resolve)
		if org, ok := resolve(one(e["organizer"])).(node); ok && text(org["name"]) != d.VenueName {
			set(&d.Company, text(org["name"]))
		}
		if w, ok := resolve(one(e["workPerformed"])).(node); ok {
			set(&d.WorkTitle, text(w["name"]))
			addPeople(d, w["author"], "author", resolve)
		}
	}
	for _, w := range works {
		addPeople(d, w["author"], "author", resolve)
		addPeople(d, w["director"], "director", resolve)
	}
	return true
}

func hasType(n node, types ...string) bool {
	var ts []string
	switch t := n["@type"].(type) {
	case string:
		ts = []string{t}
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok {
				ts = append(ts, s)
			}
		}
	}
	for _, t := range ts {
		t = t[strings.LastIndexAny(t, "/:")+1:]
		for _, want := range types {
			if t == want || (want == "Event" && strings.HasSuffix(t, "Event")) {
				return true
			}
		}
	}
	return false
}

// one returns the first element of a list, or the value itself.
func one(v any) any {
	if l, ok := v.([]any); ok {
		if len(l) == 0 {
			return nil
		}
		return l[0]
	}
	return v
}

// text reads a string-like JSON-LD value.
func text(v any) string {
	switch v := one(v).(type) {
	case string:
		return clean(v)
	case node:
		if s, ok := v["@value"].(string); ok {
			return clean(s)
		}
		if s, ok := v["name"].(string); ok {
			return clean(s)
		}
	}
	return ""
}

func imageURL(v any, resolve func(any) any) string {
	switch v := resolve(one(v)).(type) {
	case string:
		return v
	case node:
		for _, k := range []string{"url", "contentUrl"} {
			if s, ok := v[k].(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// date keeps the YYYY-MM-DD part of an ISO date or datetime.
func date(s string) string {
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		return s[:10]
	}
	return ""
}

// addPeople adds Person nodes as credits; a group becomes the company.
func addPeople(d *Draft, v any, role string, resolve func(any) any) {
	list, ok := v.([]any)
	if !ok {
		list = []any{v}
	}
	for _, e := range list {
		switch p := resolve(e).(type) {
		case []any:
			addPeople(d, p, role, resolve) // Festik nests lists: performer: [[...]]
		case string:
			addCredit(d, Credit{Name: clean(p), Role: role})
		case node:
			name := text(p["name"])
			if hasType(p, "PerformingGroup", "Organization", "TheaterGroup") {
				set(&d.Company, name)
				continue
			}
			addCredit(d, Credit{Name: name, Role: role})
		}
	}
}

// addCredit appends a credit unless it is empty or already there.
func addCredit(d *Draft, c Credit) {
	if c.Name == "" {
		return
	}
	for _, e := range d.Credits {
		if e.Role == c.Role && strings.EqualFold(e.Name, c.Name) {
			return
		}
	}
	d.Credits = append(d.Credits, c)
}

// htmlText strips tags from a description that embeds HTML.
func htmlText(s string) string {
	if !strings.Contains(s, "<") {
		return s
	}
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
			b.WriteByte(' ')
		case !in:
			b.WriteRune(r)
		}
	}
	return clean(b.String())
}
